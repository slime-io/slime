package util

import (
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// MessageToStruct converts golang proto msg to struct
func MessageToStruct(msg proto.Message) (*structpb.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}
	bs, err := protojson.Marshal(msg)
	if err != nil {
		return nil, err
	}
	pbs := &structpb.Struct{}
	if err := protojson.Unmarshal(bs, pbs); err != nil {
		return nil, err
	}
	return pbs, nil
}

func ProtoToMap(pb proto.Message) (map[string]interface{}, error) {
	if bs, err := protojson.Marshal(pb); err == nil {
		var mapResult map[string]interface{}
		// 使用 json.Unmarshal(data []byte, v interface{})进行转换,返回 error 信息
		if err := json.Unmarshal(bs, &mapResult); err == nil {
			return mapResult, nil
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
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
