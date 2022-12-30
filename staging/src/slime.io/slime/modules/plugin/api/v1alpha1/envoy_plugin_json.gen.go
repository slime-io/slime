// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: envoy_plugin.proto

package v1alpha1

import (
	bytes "bytes"
	fmt "fmt"
	github_com_gogo_protobuf_jsonpb "github.com/gogo/protobuf/jsonpb"
	proto "github.com/gogo/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// MarshalJSON is a custom marshaler for WorkloadSelector
func (this *WorkloadSelector) MarshalJSON() ([]byte, error) {
	str, err := EnvoyPluginMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for WorkloadSelector
func (this *WorkloadSelector) UnmarshalJSON(b []byte) error {
	return EnvoyPluginUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

// MarshalJSON is a custom marshaler for EnvoyPluginSpec
func (this *EnvoyPluginSpec) MarshalJSON() ([]byte, error) {
	str, err := EnvoyPluginMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for EnvoyPluginSpec
func (this *EnvoyPluginSpec) UnmarshalJSON(b []byte) error {
	return EnvoyPluginUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

// MarshalJSON is a custom marshaler for EnvoyPluginSpec_Listener
func (this *EnvoyPluginSpec_Listener) MarshalJSON() ([]byte, error) {
	str, err := EnvoyPluginMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for EnvoyPluginSpec_Listener
func (this *EnvoyPluginSpec_Listener) UnmarshalJSON(b []byte) error {
	return EnvoyPluginUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

var (
	EnvoyPluginMarshaler   = &github_com_gogo_protobuf_jsonpb.Marshaler{}
	EnvoyPluginUnmarshaler = &github_com_gogo_protobuf_jsonpb.Unmarshaler{AllowUnknownFields: true}
)
