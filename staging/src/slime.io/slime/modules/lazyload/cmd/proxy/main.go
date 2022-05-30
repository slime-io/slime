package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"slime.io/slime/modules/lazyload/pkg/proxy"
)

const (
	EnvWormholePorts = "WORMHOLE_PORTS"
	EnvProbePort     = "PROBE_PORT"
	EnvLogLevel      = "LOG_LEVEL"
)

func main() {
	// set log config
	logLevel := os.Getenv(EnvLogLevel)
	if logLevel == "" {
		logLevel = "info"
	}
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		os.Exit(1)
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.RFC3339,
	})

	// start health check server
	probePort := ":" + os.Getenv(EnvProbePort)
	go func() {
		handler := &proxy.HealthzProxy{}
		log.Println("Starting health check on", probePort)
		if err := http.ListenAndServe(probePort, handler); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()

	// start multi ports defined in WormholePorts
	wormholePorts := os.Getenv(EnvWormholePorts)
	wormholePortsArr := strings.Split(wormholePorts, ",")
	var whPorts []int
	for _, whp := range wormholePortsArr {
		if whp == probePort {
			log.Errorf("wormholePort can not be %s, which is reserved for health check", probePort)
			os.Exit(1)
		}

		p, err := strconv.Atoi(whp)
		if err != nil {
			log.Errorf("wrong wormholePort value %s", whp)
			os.Exit(1)
		}
		whPorts = append(whPorts, p)
	}

	var wg sync.WaitGroup
	for _, whPort := range whPorts {
		wg.Add(1)
		handler := &proxy.Proxy{WormholePort: whPort}
		go func(whPort int) {
			log.Println("Starting proxy on", "0.0.0.0"+":"+strconv.Itoa(whPort))
			if err := http.ListenAndServe("0.0.0.0"+":"+strconv.Itoa(whPort), handler); err != nil {
				log.Fatal("Proxy ListenAndServe error:", err)
			}
			wg.Done()
		}(whPort)
	}

	wg.Wait()
	log.Infof("All servers exited.")
}
