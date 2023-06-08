package util

import (
	"flag"
	"os"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
)

const (
	slimeLogLevel  = "info"
	slimeKLogLevel = 5
	slimeILogLevel = "warn"
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

type AnyMessage struct {
	RawJson []byte
}

func (a *AnyMessage) Reset() {
}

func (a *AnyMessage) String() string {
	return ""
}

func (a *AnyMessage) ProtoMessage() {
}

func (a *AnyMessage) UnmarshalJSONPB(_ *jsonpb.Unmarshaler, data []byte) error {
	a.RawJson = append([]byte{}, data...)
	return nil
}

func (a *AnyMessage) MarshalJSONPB(_ *jsonpb.Marshaler) ([]byte, error) {
	return a.RawJson, nil
}
