package util

import (
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type ProtoJSONOpts func(*protojson.MarshalOptions)

func UseProtoNames(useProtoNames bool) ProtoJSONOpts {
	return func(opts *protojson.MarshalOptions) {
		opts.UseProtoNames = useProtoNames
	}
}

func MessageToStructWithOpts(msg proto.Message, opts ...ProtoJSONOpts) (*structpb.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}
	mo := &protojson.MarshalOptions{}
	for _, opt := range opts {
		opt(mo)
	}
	bs, err := mo.Marshal(msg)
	if err != nil {
		return nil, err
	}
	pbs := &structpb.Struct{}
	if err := protojson.Unmarshal(bs, pbs); err != nil {
		return nil, err
	}
	return pbs, nil
}

// MessageToStruct converts golang proto msg to struct
func MessageToStruct(msg proto.Message) (*structpb.Struct, error) {
	return MessageToStructWithOpts(msg)
}

func ProtoToMapWithOpts(pb proto.Message, opts ...ProtoJSONOpts) (map[string]interface{}, error) {
	mo := &protojson.MarshalOptions{}
	for _, opt := range opts {
		opt(mo)
	}
	if bs, err := mo.Marshal(pb); err == nil {
		var mapResult map[string]interface{}
		// use json.Unmarshal(data []byte, v interface{}) to convert and return error information
		if err := json.Unmarshal(bs, &mapResult); err == nil {
			return mapResult, nil
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}

}

func ProtoToMap(pb proto.Message) (map[string]interface{}, error) {
	return ProtoToMapWithOpts(pb)
}

func FromJSONMapToMessage(data interface{}, msg proto.Message) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return protojson.Unmarshal(js, msg)
}

func ToTypedStruct(typeURL string, value *structpb.Struct) *structpb.Struct {
	return &structpb.Struct{Fields: map[string]*structpb.Value{
		StructAnyAtType:  {Kind: &structpb.Value_StringValue{StringValue: TypeURLUDPATypedStruct}},
		StructAnyTypeURL: {Kind: &structpb.Value_StringValue{StringValue: typeURL}},
		StructAnyValue:   {Kind: &structpb.Value_StructValue{StructValue: value}},
	}}
}
