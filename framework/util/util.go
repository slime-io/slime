package util

import (
	"flag"
	"os"
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
	if svc, ns, ok := IsK8SService(host); !ok {
		return host
	} else {
		return svc + "." + ns + WellknownK8sSuffix
	}
}

func Fatal() {
	os.Exit(1)
}
