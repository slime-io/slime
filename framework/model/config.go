package model

const IstioRevLabel = "istio.io/rev"

func IstioRevFromLabel(l map[string]string) string {
	if l == nil {
		return ""
	}
	return l[IstioRevLabel]
}

func LabelMatchIstioRev(l map[string]string, rev string) bool {
	if l == nil {
		return true
	}

	v, ok := l[IstioRevLabel]
	if !ok || v == rev {
		return true
	}

	return false
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
