package controllers

import "github.com/gogo/protobuf/types"

func addStructField(s *types.Struct, k string, value *types.Value) *types.Struct {
	s.Fields[k] = value
	return s
}

func structToValue(st *types.Struct) *types.Value {
	return &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: st,
		},
	}
}

func stringToValue(s string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StringValue{
			StringValue: s,
		},
	}
}

type structWrapper struct {
	st *types.Struct
}

func (s *structWrapper) AddField(k string, value *types.Value) *structWrapper {
	s.st.Fields[k] = value
	return s
}

func (s *structWrapper) AddStructField(k string, st *types.Struct) *structWrapper {
	return s.AddField(k, structToValue(st))
}

func (s *structWrapper) WrapToStruct(k string) *structWrapper {
	return newStructWrapper(nil).AddStructField(k, s.st)
}

func (s *structWrapper) WrapToListValue() *types.Value {
	return &types.Value{
		Kind: &types.Value_ListValue{
			ListValue: &types.ListValue{
				Values: []*types.Value{
					structToValue(s.st),
				},
			},
		},
	}
}

func (s *structWrapper) AddStringField(k string, v string) *structWrapper {
	return s.AddField(k, stringToValue(v))
}

func (s *structWrapper) Get() *types.Struct {
	return s.st
}

func newStructWrapper(st *types.Struct) *structWrapper {
	if st == nil {
		st = &types.Struct{Fields: map[string]*types.Value{}}
	}
	return &structWrapper{st: st}
}

func fieldToStruct(k string, value *types.Value) *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			k: value,
		},
	}
}

func wrapStructToStruct(k string, st *types.Struct) *types.Struct {
	return fieldToStruct(k, structToValue(st))
}

func wrapStructToStructWrapper(k string, st *types.Struct) *structWrapper {
	return newStructWrapper(nil).AddStructField(k, st)
}
