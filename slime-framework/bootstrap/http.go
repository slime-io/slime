package bootstrap

import (
	"github.com/sirupsen/logrus"
	"net/http"
)

var log = logrus.WithField("handler","health-probes")

func HealthCheckRegister() {
	// TODO - handle readyzPaths and livezPaths will be used when many modules in one depoloyment
}

func livezHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("Healthy!")); err != nil {
			log.Errorf("livez probe error, %+v",err)
		}
	})
}

func readyzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO - Add proper readiness check logic
		if _, err := w.Write([]byte("Healthy!")); err != nil {
			log.Errorf("readyz probe error, %+v",err)
		}
	})
}

func HealthCheckStart() {
	addr := ":8081"
	mux := http.NewServeMux()

	mux.Handle("/modules/livez", livezHandler())
	mux.Handle("/modules/readyz", readyzHandler())

	log.Infof("health check server is starting to listen %s",addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Errorf("health check server starts error, %+v",err)
	}
}
