package bootstrap

import (
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
)

var setupLog = ctrl.Log.WithName("setup")

func HealthCheckRegister() {
	// TODO - handle when many modules in one depoloyment
}

func livezHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte("Healthy!")); err != nil {
			setupLog.Error(err, "livez probe error")
		}
	})
}

func readyzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO - Add proper readiness check logic
		if _, err := w.Write([]byte("Healthy!")); err != nil {
			setupLog.Error(err, "readyz probe error")
		}
	})
}

func HealthCheckStart() {
	addr := ":8081"
	mux := http.NewServeMux()

	mux.Handle("/modules/livez", livezHandler())
	mux.Handle("/modules/readyz", readyzHandler())

	setupLog.Info("health check server is starting to listen", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		setupLog.Error(err, "health check server starts error")
	}
}
