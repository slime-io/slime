package util

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ghodss/yaml"
	gogojsonpb "github.com/gogo/protobuf/jsonpb"
	gogoproto "github.com/gogo/protobuf/proto"
	gogotypes "github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/jsonpb"
	"github.com/hashicorp/go-multierror"
	yaml2 "gopkg.in/yaml.v2"
	"io"
	"reflect"
)

// MessageToStruct
// Deprecated
// WARN:
// the callers which use this func to convert envoy api to gogo struct should mirgate to MessageToGogoStruct and do test
func MessageToStruct(msg gogoproto.Message) (*gogotypes.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}

	buf := &bytes.Buffer{}
	if err := (&gogojsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		return nil, err
	}

	pbs := &gogotypes.Struct{}
	if err := gogojsonpb.Unmarshal(buf, pbs); err != nil {
		return nil, err
	}
	return pbs, nil
}

// MessageToGogoStruct converts golang proto msg to gogo struct
func MessageToGogoStruct(msg gogoproto.Message) (*gogotypes.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}

	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		return nil, err
	}

	pbs := &gogotypes.Struct{}
	if err := gogojsonpb.Unmarshal(buf, pbs); err != nil {
		return nil, err
	}
	return pbs, nil
}

func ProtoToMap(pb gogoproto.Message) (map[string]interface{}, error) {
	m := &gogojsonpb.Marshaler{}
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

func Make(messageName string) (gogoproto.Message, error) {
	pbt := gogoproto.MessageType(messageName)
	if pbt == nil {
		return nil, fmt.Errorf("unknown type %q", messageName)
	}
	return reflect.New(pbt.Elem()).Interface().(gogoproto.Message), nil
}

func FromJSONMapToMessage(data interface{}, msg gogoproto.Message) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return ApplyJSON(bytes.NewReader(js), msg)
}

func FromJSONMap(messageName string, data interface{}) (gogoproto.Message, error) {
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
func FromYAML(messageName string, yml string) (gogoproto.Message, error) {
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
func ApplyYAML(yml string, pb gogoproto.Message) error {
	js, err := yaml.YAMLToJSON([]byte(yml))
	if err != nil {
		return err
	}
	return ApplyJSON(bytes.NewReader(js), pb)
}

// ApplyJSON unmarshals a JSON string into a proto message.
func ApplyJSON(r io.Reader, pb gogoproto.Message) error {
	m := gogojsonpb.Unmarshaler{AllowUnknownFields: true}
	return m.Unmarshal(r, pb)
}

// StructToMessage decodes a protobuf Message from a Struct.
func StructToMessage(pbst *gogotypes.Struct, out gogoproto.Message) error {
	if pbst == nil {
		return errors.New("nil struct")
	}

	buf := &bytes.Buffer{}
	if err := (&gogojsonpb.Marshaler{OrigName: true}).Marshal(buf, pbst); err != nil {
		return err
	}

	return gogojsonpb.Unmarshal(buf, out)
}

func ToTypedStruct(typeURL string, value *gogotypes.Struct) *gogotypes.Struct {
	return &gogotypes.Struct{Fields: map[string]*gogotypes.Value{
		StructAnyAtType:  {Kind: &gogotypes.Value_StringValue{StringValue: TypeURLUDPATypedStruct}},
		StructAnyTypeURL: {Kind: &gogotypes.Value_StringValue{StringValue: typeURL}},
		StructAnyValue:   {Kind: &gogotypes.Value_StructValue{StructValue: value}},
	}}
}
