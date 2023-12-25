package controllers

import (
	gogotypes "github.com/gogo/protobuf/types"
	"google.golang.org/protobuf/types/known/structpb"
)

func addStructField(s *structpb.Struct, k string, value *structpb.Value) *structpb.Struct {
	s.Fields[k] = value
	return s
}

func structToValue(st *structpb.Struct) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StructValue{
			StructValue: st,
		},
	}
}

func stringToValue(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

type structWrapper struct {
	st *structpb.Struct
}

func (s *structWrapper) AddField(k string, value *structpb.Value) *structWrapper {
	s.st.Fields[k] = value
	return s
}

func (s *structWrapper) AddStructField(k string, st *structpb.Struct) *structWrapper {
	return s.AddField(k, structToValue(st))
}

func (s *structWrapper) WrapToStruct(k string) *structWrapper {
	return newStructWrapper(nil).AddStructField(k, s.st)
}

func (s *structWrapper) WrapToListValue() *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_ListValue{
			ListValue: &structpb.ListValue{
				Values: []*structpb.Value{
					structToValue(s.st),
				},
			},
		},
	}
}

func (s *structWrapper) AddStringField(k string, v string) *structWrapper {
	return s.AddField(k, stringToValue(v))
}

func (s *structWrapper) Get() *structpb.Struct {
	return s.st
}

func newStructWrapper(st *structpb.Struct) *structWrapper {
	if st == nil {
		st = &structpb.Struct{Fields: map[string]*structpb.Value{}}
	}
	return &structWrapper{st: st}
}

func fieldToStruct(k string, value *structpb.Value) *structpb.Struct {
	return &structpb.Struct{
		Fields: map[string]*structpb.Value{
			k: value,
		},
	}
}

func wrapStructToStruct(k string, st *structpb.Struct) *structpb.Struct {
	return fieldToStruct(k, structToValue(st))
}

func wrapStructToStructWrapper(k string, st *structpb.Struct) *structWrapper {
	return newStructWrapper(nil).AddStructField(k, st)
}

func gogoStructToStruct(gogo *gogotypes.Struct) *structpb.Struct {
	// transition function, no need to implement, after the api is migrated to google proto, it is no longer needed
	return nil
}

func structToGogoStruct(st *structpb.Struct) *gogotypes.Struct {
	// transition function, no need to implement, after the api is migrated to google proto, it is no longer needed
	return nil
}

func structValueToGogoStructValue(value *structpb.Value) *gogotypes.Value {
	// transition function, no need to implement, after the api is migrated to google proto, it is no longer needed
	return nil
}

func gogoStructValueToStructValue(value *gogotypes.Value) *structpb.Value {
	// transition function, no need to implement, after the api is migrated to google proto, it is no longer needed
	return nil
}
