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
)

const (
	HeaderSourceNs = "Slime-Source-Ns"
	HeaderOrigDest = "Slime-Orig-Dest"
)

type HealthzProxy struct{}

func (p *HealthzProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// health check
	if req.RequestURI == "/healthz/live" || req.RequestURI == "/healthz/ready" {
		w.Write([]byte("Healthy!"))
		return
	}
}

type Proxy struct {
	WormholePort int
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var (
		reqCtx               = req.Context()
		reqHost              = req.Host
		origDest, origDestIp string
		origDestPort         = p.WormholePort
	)
	log.Debugf("proxy received request, reqHost: %s", reqHost)

	// try to complete short name
	if values := req.Header[HeaderSourceNs]; len(values) > 0 && values[0] != "" {
		req.Header.Del(HeaderSourceNs)
		if !strings.Contains(reqHost, ".") {
			// short name
			ns := values[0]
			if idx := strings.LastIndex(reqHost, ":"); idx >= 0 {
				reqHost = fmt.Sprintf("%s.%s%s", reqHost[:idx], ns, reqHost[idx:])
			} else {
				reqHost = fmt.Sprintf("%s.%s", reqHost, ns)
			}
		}
		log.Debugf("handle request header [Slime-Source-Ns]: %s", values[0])
	}

	if values := req.Header[HeaderOrigDest]; len(values) > 0 {
		origDest = values[0]
		req.Header.Del(HeaderOrigDest)

		if idx := strings.LastIndex(origDest, ":"); idx >= 0 {
			origDestIp = origDest[:idx]
			if v, err := strconv.Atoi(origDest[idx+1:]); err != nil {
				http.Error(w, fmt.Sprintf("invalid header %s value: %s", HeaderOrigDest, origDest), http.StatusBadRequest)
				return
			} else {
				origDestPort = v
			}
		} else {
			origDestIp = origDest
		}
		log.Debugf("handle request header [Slime-Orig-Dest]: %s", values[0])
	}

	if origDest == "" {
		if idx := strings.LastIndex(reqHost, ":"); idx >= 0 {
			origDestIp = reqHost[:idx]
		} else {
			origDestIp = reqHost
		}
	}
	log.Debugf("proxy forward request to: %s:%d", origDestIp, origDestPort)

	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	req.URL.Host = reqHost
	req.Host = reqHost
	req.RequestURI = ""
	newCtx, _ := context.WithCancel(reqCtx)
	req = req.WithContext(newCtx)

	dialer := &net.Dialer{
		// Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			addr = fmt.Sprintf("%s:%d", origDestIp, origDestPort)
			return dialer.DialContext(ctx, network, addr)
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
