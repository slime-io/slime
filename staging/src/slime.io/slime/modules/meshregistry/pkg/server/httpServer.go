package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	cmap "github.com/orcaman/concurrent-map"
	"k8s.io/kube-openapi/pkg/common"

	"slime.io/slime/modules/meshregistry/pkg/mcpoverxds"
	"slime.io/slime/modules/meshregistry/pkg/monitoring"
	"slime.io/slime/modules/meshregistry/pkg/util/cache"
)

type HttpServer struct {
	addr                string
	mux                 *http.ServeMux
	xdsReady            bool
	sourceReady         bool
	sourceReadyMsg      string
	httpPathHandler     common.PathHandler
	sources             cmap.ConcurrentMap
	lock                sync.Mutex
	mcpController       *mcpoverxds.McpController
	startWG             *sync.WaitGroup
	sourceReadyCallback func(*mcpoverxds.McpController, *sync.WaitGroup)
}

func (hs *HttpServer) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	hs.mux.HandleFunc(pattern, handler)
	if hs.httpPathHandler != nil {
		hs.httpPathHandler.Handle(pattern, http.HandlerFunc(handler))
	}
}

func (hs *HttpServer) start() {
	if hs.addr != "" {
		go func() {
			err := http.ListenAndServe(hs.addr, hs.mux) // ":8080"
			if err != nil {
				log.Errorf("http Server unexpectedly terminated: %v", err)
			}
		}()
	}

	go func() {
		t0 := time.Now()
		for {
			if len(hs.unreadySources()) == 0 {
				break
			}
			time.Sleep(time.Second)
		}

		log.Infof("unready sources empty, mark source ready and exec callback")

		hs.lock.Lock()
		hs.sourceReady = true
		hs.lock.Unlock()
		// Since the polling check method is used, the time statistics of source ready are not accurate enough,
		// which is only for reference, but marking source ready is part of meshregistry ready
		monitoring.RecordReady("sources", t0, time.Now())

		if cb := hs.sourceReadyCallback; cb != nil {
			cb(hs.mcpController, hs.startWG)
		}
	}()
}

func (hs *HttpServer) Ready() bool {
	hs.lock.Lock()
	defer hs.lock.Unlock()
	return hs.xdsReady && hs.sourceReady
}

func (hs *HttpServer) unreadySources() []string {
	var ret []string
	for k, v := range hs.sources.Items() {
		if boolValue, ok := v.(bool); ok && !boolValue {
			ret = append(ret, k)
		}
	}
	return ret
}

func (hs *HttpServer) ready() error {
	hs.lock.Lock()
	defer hs.lock.Unlock()

	var s string
	if !hs.xdsReady {
		s += " xds not ready"
	}
	if !hs.sourceReady {
		s += " source not ready"
	}

	if s == "" {
		return nil
	}
	return errors.New(s)
}

func (hs *HttpServer) handleReadyProbe(w http.ResponseWriter, _ *http.Request) {
	err := hs.ready()
	if err == nil {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error() + " " + strings.Join(hs.unreadySources(), ",")))
	}
}

func (hs *HttpServer) pc(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(cache.K8sPodCaches.GetAll(), "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal pod cahce se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func (hs *HttpServer) SourceRegistry(registry string) {
	hs.lock.Lock()
	defer hs.lock.Unlock()
	hs.sources.Set(registry, false)
	hs.sourceReady = false
}

func (hs *HttpServer) ListenerRegistry(mcpController *mcpoverxds.McpController, startWG *sync.WaitGroup, sourceReadyCallback func(*mcpoverxds.McpController, *sync.WaitGroup)) {
	hs.lock.Lock()
	defer hs.lock.Unlock()
	hs.mcpController = mcpController
	hs.startWG = startWG
	hs.sourceReadyCallback = sourceReadyCallback
}

func (hs *HttpServer) SourceReadyCallBack(registry string) {
	hs.lock.Lock()
	defer hs.lock.Unlock()
	hs.sources.Set(registry, true)
}

func (hs *HttpServer) nc(w http.ResponseWriter, _ *http.Request) {
	b, err := json.MarshalIndent(cache.K8sNodeCaches.GetAll(), "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "unable to marshal node cahce se cache: %v", err)
		return
	}
	_, _ = w.Write(b)
}

func (hs *HttpServer) SourceReadyLater(src string, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		hs.SourceReadyCallBack(src)
	}()
}

func (hs *HttpServer) handleClientsInfo(w http.ResponseWriter, request *http.Request) {
	if hs.mcpController != nil {
		hs.mcpController.HandleClientsInfo(w, request)
	}
}
