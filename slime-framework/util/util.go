package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/klog"
	"go.uber.org/zap/zapcore"

	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	cmap "github.com/orcaman/concurrent-map"
	yaml2 "gopkg.in/yaml.v2"
)

const  (
	slimeLogLevel = "info"
	slimeKLogLevel = 5
)


var fs *flag.FlagSet

// Map operation
func IsContain(farther, child map[string]string) bool {
	if len(child) > len(farther) {
		return false
	}
	for k, v := range child {
		if farther[k] != v {
			return false
		}
	}
	return true
}

func CopyMap(m1 map[string]string) map[string]string {
	ret := make(map[string]string)
	for k, v := range m1 {
		ret[k] = v
	}
	return ret
}

func MapToMapInterface(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		ks := strings.Split(k, ".")
		r, ks, err := findSubNode(ks, out)
		if err != nil {
			fmt.Printf("===err:%s", err.Error())
		}
		for k1, v1 := range createSubmap(ks, v) {
			r[k1] = v1
		}
	}
	return out
}

func createSubmap(ks []string, value string) map[string]interface{} {
	if len(ks) == 1 {
		return map[string]interface{}{
			ks[0]: value,
		}
	}
	return map[string]interface{}{
		ks[0]: createSubmap(ks[1:], value),
	}
}

func findSubNode(ks []string, root map[string]interface{}) (map[string]interface{}, []string, error) {
	if len(ks) == 0 {
		return root, ks, nil
	} else if _, ok := root[ks[0]]; !ok {
		return root, ks, nil
	} else {
		if m, ok := root[ks[0]].(map[string]interface{}); !ok {
			return nil, ks, fmt.Errorf("Leaf node reached,%v", ks)
		} else {
			return findSubNode(ks[1:], m)
		}
	}
}

// General type conversion
func MessageToStruct(msg proto.Message) (*types.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}

	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		return nil, err
	}

	pbs := &types.Struct{}
	if err := jsonpb.Unmarshal(buf, pbs); err != nil {
		return nil, err
	}

	return pbs, nil
}

func ProtoToMap(pb proto.Message) (map[string]interface{}, error) {
	m := &jsonpb.Marshaler{}
	var buf bytes.Buffer
	if err := m.Marshal(&buf, pb); err == nil {
		var mapResult map[string]interface{}
		// 使用 json.Unmarshal(data []byte, v interface{})进行转换,返回 error 信息
		if err := json.Unmarshal(buf.Bytes(), &mapResult); err == nil {
			return mapResult, nil
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}

func Make(messageName string) (proto.Message, error) {
	pbt := proto.MessageType(messageName)
	if pbt == nil {
		return nil, fmt.Errorf("unknown type %q", messageName)
	}
	return reflect.New(pbt.Elem()).Interface().(proto.Message), nil
}

func FromJSONMap(messageName string, data interface{}) (proto.Message, error) {
	// Marshal to YAML bytes
	str, err := yaml2.Marshal(data)
	if err != nil {
		return nil, err
	}
	out, err := FromYAML(messageName, string(str))
	if err != nil {
		return nil, multierror.Prefix(err, fmt.Sprintf("YAML decoding error: %v", string(str)))
	}
	return out, nil
}

// FromYAML converts a canonical YAML to a proto message
func FromYAML(messageName string, yml string) (proto.Message, error) {
	pb, err := Make(messageName)
	if err != nil {
		return nil, err
	}
	if err = ApplyYAML(yml, pb); err != nil {
		return nil, err
	}
	return pb, nil
}

// ApplyYAML unmarshals a YAML string into a proto message.
// Unknown fields are allowed.
func ApplyYAML(yml string, pb proto.Message) error {
	js, err := yaml.YAMLToJSON([]byte(yml))
	if err != nil {
		return err
	}
	return ApplyJSON(string(js), pb)
}

// ApplyJSON unmarshals a JSON string into a proto message.
func ApplyJSON(js string, pb proto.Message) error {
	reader := strings.NewReader(js)
	m := jsonpb.Unmarshaler{}
	if err := m.Unmarshal(reader, pb); err != nil {
		// log.Debugf("Failed to decode proto: %q. Trying decode with AllowUnknownFields=true", err)
		m.AllowUnknownFields = true
		reader.Reset(js)
		return m.Unmarshal(reader, pb)
	}
	return nil
}

// StructToMessage decodes a protobuf Message from a Struct.
func StructToMessage(pbst *types.Struct, out proto.Message) error {
	if pbst == nil {
		return errors.New("nil struct")
	}

	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, pbst); err != nil {
		return err
	}

	return jsonpb.Unmarshal(buf, out)
}

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
		return host + "." + namespace + Wellkonw_K8sSuffix
	}
	if svc, ns, ok := IsK8SService(host); !ok {
		return host
	} else {
		return svc + "." + ns + Wellkonw_K8sSuffix
	}
}

// Subscribeable map
type SubcribeableMap struct {
	data           cmap.ConcurrentMap
	subscriber     []func(key string, value interface{})
	subscriberLock sync.RWMutex
}

func NewSubcribeableMap() *SubcribeableMap {
	return &SubcribeableMap{
		data:           cmap.New(),
		subscriber:     make([]func(key string, value interface{}), 0),
		subscriberLock: sync.RWMutex{},
	}
}

func (s *SubcribeableMap) Set(key string, value interface{}) {
	s.data.Set(key, value)
	s.subscriberLock.RLock()
	for _, f := range s.subscriber {
		f(key, value)
	}
	s.subscriberLock.RUnlock()
}

func (s *SubcribeableMap) Pop(key string) {
	s.data.Pop(key)
	s.subscriberLock.RLock()
	for _, f := range s.subscriber {
		f(key, nil)
	}
	s.subscriberLock.RUnlock()
}

func (s *SubcribeableMap) Get(host string) interface{} {
	if i, ok := s.data.Get(host); ok {
		return i
	}
	return nil
}

func (s *SubcribeableMap) Subscribe(subscribe func(key string, value interface{})) {
	s.subscriberLock.Lock()
	s.subscriber = append(s.subscriber, subscribe)
	s.subscriberLock.Unlock()
}

func TimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02T15:04:05.000"))
}

func InitLog(LogLevel string, KlogLevel int32) error {

	if LogLevel == "" {
		LogLevel = slimeLogLevel
	}
	if KlogLevel == 0 {
		KlogLevel = slimeKLogLevel
	}
	level, err := log.ParseLevel(LogLevel)
	if err != nil {
		return err
	} else {
		log.SetLevel(level)
		log.SetOutput(os.Stdout)
		log.SetFormatter(&log.TextFormatter{
			TimestampFormat: time.RFC3339,
		})
	}

	if KlogLevel != 0 {
		initKlog(KlogLevel)
	}
	return nil
}

func SetLevel(LogLevel string) error {
	level, err := log.ParseLevel(LogLevel)
	if err != nil {
		return err
	}
	log.SetLevel(level)
	return nil
}

// SetReportCaller sets whether the standard logger will include the calling
// method as a field, default false.
func SetReportCaller(support bool) {
	log.SetReportCaller(support)
}

func GetLevel() string {
	level := log.GetLevel()
	return level.String()
}

// initKlog while x<= KlogLevel in the klog.V("x").info("hello"), log will be record
func initKlog(KlogLevel int32) {
	fs = flag.NewFlagSet("klog",flag.ContinueOnError)
	klog.InitFlags(fs)
	SetKlogLevel(KlogLevel)
}

// SetKlogLevel Warning: not thread safe
func SetKlogLevel(number int32) {
	fs.Set("v", fmt.Sprintf("%d", number))
}

func GetKlogLevel() string {
	return fs.Lookup("v").Value.String()
}