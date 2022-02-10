package bootstrap

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strconv"

	log "github.com/sirupsen/logrus"
	"k8s.io/kube-openapi/pkg/common"

	"slime.io/slime/framework/util"
)

// PathHandler for module using
type PathHandler struct {
	mux *http.ServeMux
}

func NewPathHandler() *PathHandler {
	return &PathHandler{
		mux: http.NewServeMux(),
	}
}

func (ph PathHandler) Handle(path string, handler http.Handler) {
	ph.mux.Handle(path, handler)
}

// PrefixPathHandlerManager for module env init
type PrefixPathHandlerManager struct {
	Prefix string // module name
	common.PathHandler
}

func (m PrefixPathHandlerManager) Handle(path string, handler http.Handler) {
	m.PathHandler.Handle("/"+m.Prefix+"/"+path, handler)
}

func AuxiliaryHttpServerStart(ph *PathHandler, addr string) {
	// register
	HealthCheckRegister(ph)
	PprofRegister(ph)
	LogLevelRegister(ph)

	log.Infof("auxiliary http server is starting to listen %s", addr)
	if err := http.ListenAndServe(addr, ph.mux); err != nil {
		log.Errorf("auxiliary http server starts error, %+v", err)
	}
}

func HealthCheckRegister(ph *PathHandler) {
	ph.Handle("/modules/livez", livezHandler())
	ph.Handle("/modules/readyz", readyzHandler())
}

func HealthCheckPathRegister() {
	// TODO - handle readyzPaths and livezPaths will be used when many modules in one depoloyment
}

func livezHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("Healthy!")); err != nil {
			log.Errorf("livez probe error, %+v", err)
		}
	})
}

func readyzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO - Add proper readiness check logic
		if _, err := w.Write([]byte("Healthy!")); err != nil {
			log.Errorf("readyz probe error, %+v", err)
		}
	})
}

func PprofRegister(ph *PathHandler) {
	ph.mux.HandleFunc("/debug/pprof/", pprof.Index)
	ph.mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	ph.mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	ph.mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	ph.mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func LogLevelRegister(ph *PathHandler) {
	ph.Handle("/log/slime", slimeLogLevelHandler())
	ph.Handle("/log/k", kLogLevelHandler())
}

func slimeLogLevelHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			slimeLogLevel := util.GetLevel()
			if _, err := w.Write([]byte(fmt.Sprintf("Slime log level is %s.", slimeLogLevel))); err != nil {
				log.Errorf("write slime log level response error, %+v", err)
			}
			return
		}
		if r.Method == "PUT" || r.Method == "POST" {
			level, ok := r.URL.Query()["level"]
			if !ok || len(level) < 1 {
				log.Errorf("empty slime log level set error")
				if _, err := w.Write([]byte("Empty slime log level error!")); err != nil {
					log.Errorf("write slime log level response error, %+v", err)
				}
				return
			}
			if err := util.SetLevel(level[0]); err != nil {
				log.Errorf("wrong slime log level set error")
				if _, err := w.Write([]byte("Wrong slime log level error!")); err != nil {
					log.Errorf("write slime log level response error, %+v", err)
				}
				return
			}
			log.Infof("slime log level sets to %s successfully", level)
			if _, err := w.Write([]byte(fmt.Sprintf("Slime log level sets to %s successfully.", level))); err != nil {
				log.Errorf("write slime log level response error, %+v", err)
			}
			return
		}
		if _, err := w.Write([]byte("Wrong request method")); err != nil {
			log.Errorf("write slime log level response error, %+v", err)
		}
	})
}

func kLogLevelHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			kLogLevel := util.GetKlogLevel()
			if _, err := w.Write([]byte(fmt.Sprintf("Klog level is %s.", kLogLevel))); err != nil {
				log.Errorf("write klog level response error, %+v", err)
			}
			return
		}
		if r.Method == "PUT" || r.Method == "POST" {
			level, ok := r.URL.Query()["level"]
			if !ok || len(level) < 1 {
				log.Errorf("empty klog level set error")
				if _, err := w.Write([]byte("Empty klog level error!")); err != nil {
					log.Errorf("write klog level response error, %+v", err)
				}
				return
			}
			l, err := strconv.Atoi(level[0])
			if err != nil {
				log.Errorf("wrong klog level set error")
				if _, err := w.Write([]byte("Wrong klog level error!")); err != nil {
					log.Errorf("write klog level response error, %+v", err)
				}
				return
			}
			util.SetKlogLevel(int32(l))
			log.Infof("klog level sets to %d successfully", l)
			if _, err := w.Write([]byte(fmt.Sprintf("Klog level sets to %d successfully.", l))); err != nil {
				log.Errorf("write klog level response error, %+v", err)
			}
			return
		}
		if _, err := w.Write([]byte("Wrong request method")); err != nil {
			log.Errorf("write klog level response error, %+v", err)
		}
	})
}
