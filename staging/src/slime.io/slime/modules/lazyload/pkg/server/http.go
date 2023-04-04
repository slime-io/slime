package server

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"k8s.io/kube-openapi/pkg/common"
	"net/http"
	"slime.io/slime/framework/model/metric"
)

type Handler struct {
	HttpPathHandler common.PathHandler
	Source          metric.Source
}

func (s *Handler) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if s.HttpPathHandler != nil {
		s.HttpPathHandler.Handle(pattern, http.HandlerFunc(handler))
	}
}

// SvfResetSetting ns is needed, it will reset all svf in ns if svc is empty
// otherwise, ns/name will reset
func (s *Handler) SvfResetSetting(w http.ResponseWriter, r *http.Request) {

	ns := r.URL.Query().Get("ns")
	name := r.URL.Query().Get("name")
	if ns == "" {
		http.Error(w, "ns name is empty", http.StatusBadRequest)
		return
	}

	info := ns + "/" + name
	if err := s.Source.Reset(info); err != nil {
		http.Error(w, fmt.Sprintf("svf reset err %s", err), http.StatusInternalServerError)
		return
	}

	if _, err := w.Write([]byte("succeed")); err != nil {
		log.Errorf("reset svf %s err %s", info, err)
	}
}
