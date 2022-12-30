// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: envoy_plugin.proto

package v1alpha1

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

// `WorkloadSelector` specifies the criteria used to determine if the
// `Gateway`, `Sidecar`, or `EnvoyFilter` or `ServiceEntry`
// configuration can be applied to a proxy. The matching criteria
// includes the metadata associated with a proxy, workload instance
// info such as labels attached to the pod/VM, or any other info that
// the proxy provides to Istio during the initial handshake. If
// multiple conditions are specified, all conditions need to match in
// order for the workload instance to be selected. Currently, only
// label based selection mechanism is supported.
type WorkloadSelector struct {
	// One or more labels that indicate a specific set of pods/VMs
	// on which the configuration should be applied. The scope of
	// label search is restricted to the configuration namespace in which the
	// the resource is present.
	Labels               map[string]string `protobuf:"bytes,1,rep,name=labels,proto3" json:"labels,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *WorkloadSelector) Reset()         { *m = WorkloadSelector{} }
func (m *WorkloadSelector) String() string { return proto.CompactTextString(m) }
func (*WorkloadSelector) ProtoMessage()    {}
func (*WorkloadSelector) Descriptor() ([]byte, []int) {
	return fileDescriptor_c5a818f331e9d367, []int{0}
}
func (m *WorkloadSelector) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_WorkloadSelector.Unmarshal(m, b)
}
func (m *WorkloadSelector) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_WorkloadSelector.Marshal(b, m, deterministic)
}
func (m *WorkloadSelector) XXX_Merge(src proto.Message) {
	xxx_messageInfo_WorkloadSelector.Merge(m, src)
}
func (m *WorkloadSelector) XXX_Size() int {
	return xxx_messageInfo_WorkloadSelector.Size(m)
}
func (m *WorkloadSelector) XXX_DiscardUnknown() {
	xxx_messageInfo_WorkloadSelector.DiscardUnknown(m)
}

var xxx_messageInfo_WorkloadSelector proto.InternalMessageInfo

func (m *WorkloadSelector) GetLabels() map[string]string {
	if m != nil {
		return m.Labels
	}
	return nil
}

type EnvoyPluginSpec struct {
	WorkloadSelector *WorkloadSelector `protobuf:"bytes,9,opt,name=workload_selector,json=workloadSelector,proto3" json:"workload_selector,omitempty"`
	// route level plugin
	Route []string `protobuf:"bytes,1,rep,name=route,proto3" json:"route,omitempty"`
	// host level plugin
	Host []string `protobuf:"bytes,2,rep,name=host,proto3" json:"host,omitempty"`
	// service level plugin
	Service []string  `protobuf:"bytes,3,rep,name=service,proto3" json:"service,omitempty"`
	Plugins []*Plugin `protobuf:"bytes,4,rep,name=plugins,proto3" json:"plugins,omitempty"`
	// which gateway should use this plugin setting
	Gateway []string `protobuf:"bytes,5,rep,name=gateway,proto3" json:"gateway,omitempty"`
	// which user should use this plugin setting
	User []string `protobuf:"bytes,6,rep,name=user,proto3" json:"user,omitempty"`
	// Deprecated
	IsGroupSetting bool `protobuf:"varint,7,opt,name=isGroupSetting,proto3" json:"isGroupSetting,omitempty"`
	// listener level
	Listener             []*EnvoyPluginSpec_Listener `protobuf:"bytes,8,rep,name=listener,proto3" json:"listener,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                    `json:"-"`
	XXX_unrecognized     []byte                      `json:"-"`
	XXX_sizecache        int32                       `json:"-"`
}

func (m *EnvoyPluginSpec) Reset()         { *m = EnvoyPluginSpec{} }
func (m *EnvoyPluginSpec) String() string { return proto.CompactTextString(m) }
func (*EnvoyPluginSpec) ProtoMessage()    {}
func (*EnvoyPluginSpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_c5a818f331e9d367, []int{1}
}
func (m *EnvoyPluginSpec) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EnvoyPluginSpec.Unmarshal(m, b)
}
func (m *EnvoyPluginSpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EnvoyPluginSpec.Marshal(b, m, deterministic)
}
func (m *EnvoyPluginSpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EnvoyPluginSpec.Merge(m, src)
}
func (m *EnvoyPluginSpec) XXX_Size() int {
	return xxx_messageInfo_EnvoyPluginSpec.Size(m)
}
func (m *EnvoyPluginSpec) XXX_DiscardUnknown() {
	xxx_messageInfo_EnvoyPluginSpec.DiscardUnknown(m)
}

var xxx_messageInfo_EnvoyPluginSpec proto.InternalMessageInfo

func (m *EnvoyPluginSpec) GetWorkloadSelector() *WorkloadSelector {
	if m != nil {
		return m.WorkloadSelector
	}
	return nil
}

func (m *EnvoyPluginSpec) GetRoute() []string {
	if m != nil {
		return m.Route
	}
	return nil
}

func (m *EnvoyPluginSpec) GetHost() []string {
	if m != nil {
		return m.Host
	}
	return nil
}

func (m *EnvoyPluginSpec) GetService() []string {
	if m != nil {
		return m.Service
	}
	return nil
}

func (m *EnvoyPluginSpec) GetPlugins() []*Plugin {
	if m != nil {
		return m.Plugins
	}
	return nil
}

func (m *EnvoyPluginSpec) GetGateway() []string {
	if m != nil {
		return m.Gateway
	}
	return nil
}

func (m *EnvoyPluginSpec) GetUser() []string {
	if m != nil {
		return m.User
	}
	return nil
}

func (m *EnvoyPluginSpec) GetIsGroupSetting() bool {
	if m != nil {
		return m.IsGroupSetting
	}
	return false
}

func (m *EnvoyPluginSpec) GetListener() []*EnvoyPluginSpec_Listener {
	if m != nil {
		return m.Listener
	}
	return nil
}

type EnvoyPluginSpec_Listener struct {
	Port                 uint32   `protobuf:"varint,1,opt,name=port,proto3" json:"port,omitempty"`
	Outbound             bool     `protobuf:"varint,2,opt,name=outbound,proto3" json:"outbound,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *EnvoyPluginSpec_Listener) Reset()         { *m = EnvoyPluginSpec_Listener{} }
func (m *EnvoyPluginSpec_Listener) String() string { return proto.CompactTextString(m) }
func (*EnvoyPluginSpec_Listener) ProtoMessage()    {}
func (*EnvoyPluginSpec_Listener) Descriptor() ([]byte, []int) {
	return fileDescriptor_c5a818f331e9d367, []int{1, 0}
}
func (m *EnvoyPluginSpec_Listener) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EnvoyPluginSpec_Listener.Unmarshal(m, b)
}
func (m *EnvoyPluginSpec_Listener) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EnvoyPluginSpec_Listener.Marshal(b, m, deterministic)
}
func (m *EnvoyPluginSpec_Listener) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EnvoyPluginSpec_Listener.Merge(m, src)
}
func (m *EnvoyPluginSpec_Listener) XXX_Size() int {
	return xxx_messageInfo_EnvoyPluginSpec_Listener.Size(m)
}
func (m *EnvoyPluginSpec_Listener) XXX_DiscardUnknown() {
	xxx_messageInfo_EnvoyPluginSpec_Listener.DiscardUnknown(m)
}

var xxx_messageInfo_EnvoyPluginSpec_Listener proto.InternalMessageInfo

func (m *EnvoyPluginSpec_Listener) GetPort() uint32 {
	if m != nil {
		return m.Port
	}
	return 0
}

func (m *EnvoyPluginSpec_Listener) GetOutbound() bool {
	if m != nil {
		return m.Outbound
	}
	return false
}

func init() {
	proto.RegisterType((*WorkloadSelector)(nil), "slime.microservice.plugin.v1alpha1.WorkloadSelector")
	proto.RegisterMapType((map[string]string)(nil), "slime.microservice.plugin.v1alpha1.WorkloadSelector.LabelsEntry")
	proto.RegisterType((*EnvoyPluginSpec)(nil), "slime.microservice.plugin.v1alpha1.EnvoyPluginSpec")
	proto.RegisterType((*EnvoyPluginSpec_Listener)(nil), "slime.microservice.plugin.v1alpha1.EnvoyPluginSpec.Listener")
}

func init() { proto.RegisterFile("envoy_plugin.proto", fileDescriptor_c5a818f331e9d367) }

var fileDescriptor_c5a818f331e9d367 = []byte{
	// 414 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x92, 0xcd, 0x8a, 0xdb, 0x30,
	0x14, 0x85, 0x71, 0x32, 0x49, 0x9c, 0x1b, 0xda, 0xa6, 0x22, 0x0b, 0x91, 0x55, 0xc8, 0xa2, 0x84,
	0xa1, 0xd8, 0xcc, 0xb4, 0x8b, 0x76, 0xe8, 0xa2, 0x94, 0x0e, 0xdd, 0xcc, 0xa2, 0x28, 0x8b, 0x0e,
	0xdd, 0x04, 0xc5, 0xb9, 0x78, 0xc4, 0xc8, 0x96, 0xd1, 0x8f, 0x83, 0x9f, 0xa8, 0xef, 0xd6, 0xa7,
	0x28, 0x96, 0xec, 0xa1, 0x78, 0xd3, 0xa1, 0xbb, 0x7b, 0x0e, 0xd6, 0x39, 0x9f, 0x74, 0x0d, 0x04,
	0xcb, 0x5a, 0x35, 0x87, 0x4a, 0xba, 0x5c, 0x94, 0x49, 0xa5, 0x95, 0x55, 0x64, 0x6b, 0xa4, 0x28,
	0x30, 0x29, 0x44, 0xa6, 0x95, 0x41, 0x5d, 0x8b, 0x0c, 0x93, 0xee, 0x83, 0xfa, 0x8a, 0xcb, 0xea,
	0x81, 0x5f, 0xad, 0x57, 0xc1, 0x38, 0x14, 0xbc, 0xe4, 0x39, 0xea, 0x70, 0x72, 0xfb, 0x2b, 0x82,
	0xe5, 0x0f, 0xa5, 0x1f, 0xa5, 0xe2, 0xa7, 0x3d, 0x4a, 0xcc, 0xac, 0xd2, 0xe4, 0x1e, 0xa6, 0x92,
	0x1f, 0x51, 0x1a, 0x1a, 0x6d, 0xc6, 0xbb, 0xc5, 0xf5, 0xe7, 0xe4, 0xdf, 0xf9, 0xc9, 0x30, 0x25,
	0xb9, 0xf3, 0x11, 0xb7, 0xa5, 0xd5, 0x0d, 0xeb, 0xf2, 0xd6, 0x1f, 0x61, 0xf1, 0x97, 0x4d, 0x96,
	0x30, 0x7e, 0xc4, 0x86, 0x46, 0x9b, 0x68, 0x37, 0x67, 0xed, 0x48, 0x56, 0x30, 0xa9, 0xb9, 0x74,
	0x48, 0x47, 0xde, 0x0b, 0xe2, 0x66, 0xf4, 0x21, 0xda, 0xfe, 0x1e, 0xc3, 0xab, 0xdb, 0xf6, 0xea,
	0xdf, 0x7d, 0xf1, 0xbe, 0xc2, 0x8c, 0x70, 0x78, 0x7d, 0xee, 0x6a, 0x0f, 0xa6, 0xeb, 0xa5, 0xf3,
	0x4d, 0xb4, 0x5b, 0x5c, 0xbf, 0xff, 0x1f, 0x66, 0xb6, 0x3c, 0x0f, 0xdf, 0x62, 0x05, 0x13, 0xad,
	0x9c, 0x45, 0xff, 0x14, 0x73, 0x16, 0x04, 0x21, 0x70, 0xf1, 0xa0, 0x8c, 0xa5, 0x23, 0x6f, 0xfa,
	0x99, 0x50, 0x98, 0x75, 0x3d, 0x74, 0xec, 0xed, 0x5e, 0x92, 0xaf, 0x30, 0x0b, 0xcd, 0x86, 0x5e,
	0xf8, 0x07, 0xbd, 0x7c, 0x0e, 0x5c, 0xb8, 0x27, 0xeb, 0x8f, 0xb6, 0xf9, 0x39, 0xb7, 0x78, 0xe6,
	0x0d, 0x9d, 0x84, 0xfc, 0x4e, 0xb6, 0x34, 0xce, 0xa0, 0xa6, 0xd3, 0x40, 0xd3, 0xce, 0xe4, 0x0d,
	0xbc, 0x14, 0xe6, 0x9b, 0x56, 0xae, 0xda, 0xa3, 0xb5, 0xa2, 0xcc, 0xe9, 0x6c, 0x13, 0xed, 0x62,
	0x36, 0x70, 0xc9, 0x3d, 0xc4, 0x52, 0x18, 0x8b, 0x25, 0x6a, 0x1a, 0x7b, 0xb8, 0x4f, 0xcf, 0x81,
	0x1b, 0x6c, 0x22, 0xb9, 0xeb, 0x32, 0xd8, 0x53, 0xda, 0xfa, 0x06, 0xe2, 0xde, 0x6d, 0x09, 0x2b,
	0xa5, 0xad, 0xdf, 0xf4, 0x0b, 0xe6, 0x67, 0xb2, 0x86, 0x58, 0x39, 0x7b, 0x54, 0xae, 0x3c, 0xf9,
	0x6d, 0xc7, 0xec, 0x49, 0x7f, 0x79, 0xfb, 0xf3, 0x32, 0x40, 0x08, 0x95, 0xfa, 0x21, 0x2d, 0xd4,
	0xc9, 0x49, 0x34, 0x69, 0x00, 0x49, 0x79, 0x25, 0xd2, 0x1e, 0xe6, 0x38, 0xf5, 0xff, 0xf2, 0xbb,
	0x3f, 0x01, 0x00, 0x00, 0xff, 0xff, 0xbb, 0xd7, 0x31, 0x55, 0x1b, 0x03, 0x00, 0x00,
}
