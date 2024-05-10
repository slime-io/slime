package util

import (
	"flag"
	"strings"
)

const (
	slimeLogLevel         = "info"
	slimeKLogLevel        = 5
	slimeDefaultILogLevel = "info"
	slimeDefaultScopeName = "default"
)

var fs *flag.FlagSet

// K8S operation
func IsK8SService(host string) (string, string, bool) {
	ss := strings.Split(host, ".")
	if len(ss) != 2 && len(ss) != 5 {
		return "", "", false
	}
	return ss[0], ss[1], true
}

func UnityHost(host string, namespace string) string {
	if len(strings.Split(host, ".")) == 1 {
		return host + "." + namespace + WellknownK8sSuffix
	}
	svc, ns, ok := IsK8SService(host)
	if !ok {
		return host
	}
	return svc + "." + ns + WellknownK8sSuffix
}
