package bootstrap

import (
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/http/pprof"
)

func AuxiliaryHttpServerStart() {
	addr := ":8081"
	mux := http.NewServeMux()

	//register
	HealthCheckRegister(mux)
	PprofRegister(mux)

	log.Infof("auxiliary http server is starting to listen %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Errorf("auxiliary http server starts error, %+v", err)
	}
}

func HealthCheckRegister(mux *http.ServeMux) {
	mux.Handle("/modules/livez", livezHandler())
	mux.Handle("/modules/readyz", readyzHandler())
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

func PprofRegister(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
