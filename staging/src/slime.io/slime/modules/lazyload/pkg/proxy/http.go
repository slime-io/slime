package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
)

const (
	HeaderSourceNs = "Slime-Source-Ns"
	HeaderOrigDest = "Slime-Orig-Dest"

	defaultHTTPPort = 80
)

type HealthzProxy struct{}

func (p *HealthzProxy) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	// health check, return 200 directly
}

type Proxy struct {
	WormholePort                int
	SvcCache                    *Cache
	WormholePortPriorToHostPort bool
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var (
		reqCtx           = req.Context()
		reqHost          = req.Host
		origDest, destIp string
		destPort         = p.WormholePort
	)
	log.Debugf("proxy received request, reqHost: %s", reqHost)

	if srcNs := req.Header.Get(HeaderSourceNs); srcNs != "" {
		req.Header.Del(HeaderSourceNs)

		// we do not sure if reqHost is k8s short name or no ns service
		// so k8s svc will be extended/searched first
		// otherwise original reqHost is used

		if !strings.Contains(reqHost, ".") {
			// short name
			var (
				svcName = reqHost
				port    string
			)

			// if host has port info, extract it
			idx := strings.LastIndex(reqHost, ":")
			if idx >= 0 {
				svcName = reqHost[:idx]
				port = reqHost[idx+1:]
			}

			nn := types.NamespacedName{
				Namespace: srcNs,
				Name:      svcName,
			}

			// it means svc controller is disabled when SvcCache is nil,
			// so, all short domain should add ns info

			if p.SvcCache == nil || p.SvcCache.Exist(nn) {
				if idx >= 0 {
					// add port info
					reqHost = fmt.Sprintf("%s.%s:%s", nn.Name, nn.Namespace, port)
				} else {
					reqHost = fmt.Sprintf("%s.%s", nn.Name, nn.Namespace)
				}
			}
		}

		log.Debugf("handle request header [Slime-Source-Ns]: %s", srcNs)
	}

	if origDest = req.Header.Get(HeaderOrigDest); origDest != "" {
		req.Header.Del(HeaderOrigDest)

		if idx := strings.LastIndex(origDest, ":"); idx >= 0 {
			destIp = origDest[:idx]
			port, err := strconv.Atoi(origDest[idx+1:])
			if err != nil {
				errMsg := fmt.Sprintf("invalid header %s value: %s", HeaderOrigDest, origDest)
				http.Error(w, errMsg, http.StatusBadRequest)
				return
			}
			destPort = port
		} else {
			destIp = origDest
		}
		log.Debugf("handle request header [Slime-Orig-Dest]: %s", origDest)
	} else {
		var reqPort int
		if idx := strings.LastIndex(reqHost, ":"); idx >= 0 {
			destIp = reqHost[:idx]
			v, err := strconv.Atoi(reqHost[idx+1:])
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid host %s value: %s", reqHost, reqHost), http.StatusBadRequest)
				return
			}
			reqPort = v
		} else {
			destIp = reqHost
			reqPort = defaultHTTPPort
		}

		if !p.WormholePortPriorToHostPort {
			destPort = reqPort
		}
	}

	log.Debugf("proxy forward request to: %s:%d", destIp, destPort)

	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	req.URL.Host = reqHost
	req.Host = reqHost
	req.RequestURI = ""
	req = req.WithContext(reqCtx)

	dialer := &net.Dialer{
		// Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			realReqAddr := fmt.Sprintf("%s:%d", destIp, destPort)
			return dialer.DialContext(ctx, network, realReqAddr)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
	}

	resp, err := client.Do(req)
	if err != nil {
		select {
		case <-reqCtx.Done():
		default:
			log.Infof("do req get err %v", err)
			http.Error(w, "", http.StatusInternalServerError)
		}
		return
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
