// Code generated by protoc-gen-jsonshim. DO NOT EDIT.
package config

import (
	bytes "bytes"
	jsonpb "github.com/golang/protobuf/jsonpb"
)

// MarshalJSON is a custom marshaler for Fence
func (this *Fence) MarshalJSON() ([]byte, error) {
	str, err := FenceModuleMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for Fence
func (this *Fence) UnmarshalJSON(b []byte) error {
	return FenceModuleUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

// MarshalJSON is a custom marshaler for Dispatch
func (this *Dispatch) MarshalJSON() ([]byte, error) {
	str, err := FenceModuleMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for Dispatch
func (this *Dispatch) UnmarshalJSON(b []byte) error {
	return FenceModuleUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

// MarshalJSON is a custom marshaler for DomainAlias
func (this *DomainAlias) MarshalJSON() ([]byte, error) {
	str, err := FenceModuleMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for DomainAlias
func (this *DomainAlias) UnmarshalJSON(b []byte) error {
	return FenceModuleUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

var (
	FenceModuleMarshaler   = &jsonpb.Marshaler{}
	FenceModuleUnmarshaler = &jsonpb.Unmarshaler{AllowUnknownFields: true}
)
