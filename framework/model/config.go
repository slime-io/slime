package model

import (
	"os"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const IstioRevLabel = "istio.io/rev"

var passthroughLabelPattern = regexp.MustCompile(os.Getenv("SLIME_PASS_LBL_REG"))

func IstioRevFromLabel(l map[string]string) string {
	if l == nil {
		return ""
	}
	return l[IstioRevLabel]
}

func PatchIstioRevLabel(lbls *map[string]string, rev string) {
	if rev == "" {
		return
	}
	if *lbls == nil {
		*lbls = map[string]string{}
	}

	(*lbls)[IstioRevLabel] = rev
}

func PassthroughLabels(lbls *map[string]string, from map[string]string) {
	if passthroughLabelPattern.String() == "" {
		return
	}
	for k, v := range from {
		if !passthroughLabelPattern.MatchString(k) {
			continue
		}
		if *lbls == nil {
			*lbls = map[string]string{}
		}
		(*lbls)[k] = v
	}
}

func PatchObjectMeta(dst, src *metav1.ObjectMeta) {
	if src == nil {
		return
	}

	if dst == nil {
		*dst = metav1.ObjectMeta{}
	}

	PassthroughLabels(&dst.Labels, src.Labels)
}
