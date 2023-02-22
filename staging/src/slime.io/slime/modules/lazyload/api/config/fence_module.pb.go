// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: fence_module.proto

package config

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

type Fence struct {
	// service ports enable lazyload
	WormholePort []string `protobuf:"bytes,1,rep,name=wormholePort,proto3" json:"wormholePort,omitempty"`
	// whether enable ServiceFence auto generating
	// default value is false
	AutoFence bool `protobuf:"varint,2,opt,name=autoFence,proto3" json:"autoFence,omitempty"`
	// the namespace list which enable lazyload
	Namespace []string `protobuf:"bytes,3,rep,name=namespace,proto3" json:"namespace,omitempty"`
	// custom outside dispatch traffic rules
	Dispatches []*Dispatch `protobuf:"bytes,4,rep,name=dispatches,proto3" json:"dispatches,omitempty"`
	// can convert to one or many domain alias rules
	DomainAliases []*DomainAlias `protobuf:"bytes,5,rep,name=domainAliases,proto3" json:"domainAliases,omitempty"`
	// default behavior of create fence or not when autoFence is true
	// default value is false
	DefaultFence bool `protobuf:"varint,6,opt,name=defaultFence,proto3" json:"defaultFence,omitempty"`
	// whether enable http service port auto management
	// default value is false
	AutoPort bool `protobuf:"varint,7,opt,name=autoPort,proto3" json:"autoPort,omitempty"`
	// specify the ns of global-siecar, same as slimeNamespace by default
	ClusterGsNamespace string `protobuf:"bytes,8,opt,name=clusterGsNamespace,proto3" json:"clusterGsNamespace,omitempty"`
	// specify label key and alias to generate sf
	FenceLabelKeyAlias string `protobuf:"bytes,9,opt,name=fenceLabelKeyAlias,proto3" json:"fenceLabelKeyAlias,omitempty"`
	// enableShortDomain
	EnableShortDomain    bool     `protobuf:"varint,10,opt,name=enableShortDomain,proto3" json:"enableShortDomain,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Fence) Reset()         { *m = Fence{} }
func (m *Fence) String() string { return proto.CompactTextString(m) }
func (*Fence) ProtoMessage()    {}
func (*Fence) Descriptor() ([]byte, []int) {
	return fileDescriptor_8eebc4b237a55c9b, []int{0}
}
func (m *Fence) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Fence.Unmarshal(m, b)
}
func (m *Fence) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Fence.Marshal(b, m, deterministic)
}
func (m *Fence) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Fence.Merge(m, src)
}
func (m *Fence) XXX_Size() int {
	return xxx_messageInfo_Fence.Size(m)
}
func (m *Fence) XXX_DiscardUnknown() {
	xxx_messageInfo_Fence.DiscardUnknown(m)
}

var xxx_messageInfo_Fence proto.InternalMessageInfo

func (m *Fence) GetWormholePort() []string {
	if m != nil {
		return m.WormholePort
	}
	return nil
}

func (m *Fence) GetAutoFence() bool {
	if m != nil {
		return m.AutoFence
	}
	return false
}

func (m *Fence) GetNamespace() []string {
	if m != nil {
		return m.Namespace
	}
	return nil
}

func (m *Fence) GetDispatches() []*Dispatch {
	if m != nil {
		return m.Dispatches
	}
	return nil
}

func (m *Fence) GetDomainAliases() []*DomainAlias {
	if m != nil {
		return m.DomainAliases
	}
	return nil
}

func (m *Fence) GetDefaultFence() bool {
	if m != nil {
		return m.DefaultFence
	}
	return false
}

func (m *Fence) GetAutoPort() bool {
	if m != nil {
		return m.AutoPort
	}
	return false
}

func (m *Fence) GetClusterGsNamespace() string {
	if m != nil {
		return m.ClusterGsNamespace
	}
	return ""
}

func (m *Fence) GetFenceLabelKeyAlias() string {
	if m != nil {
		return m.FenceLabelKeyAlias
	}
	return ""
}

func (m *Fence) GetEnableShortDomain() bool {
	if m != nil {
		return m.EnableShortDomain
	}
	return false
}

// The general idea is to assign different default traffic to different targets
// for correct processing by means of domain matching.
type Dispatch struct {
	// dispatch rule name
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// domain matching contents
	Domains []string `protobuf:"bytes,2,rep,name=domains,proto3" json:"domains,omitempty"`
	// target cluster
	Cluster              string   `protobuf:"bytes,3,opt,name=cluster,proto3" json:"cluster,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Dispatch) Reset()         { *m = Dispatch{} }
func (m *Dispatch) String() string { return proto.CompactTextString(m) }
func (*Dispatch) ProtoMessage()    {}
func (*Dispatch) Descriptor() ([]byte, []int) {
	return fileDescriptor_8eebc4b237a55c9b, []int{1}
}
func (m *Dispatch) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Dispatch.Unmarshal(m, b)
}
func (m *Dispatch) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Dispatch.Marshal(b, m, deterministic)
}
func (m *Dispatch) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Dispatch.Merge(m, src)
}
func (m *Dispatch) XXX_Size() int {
	return xxx_messageInfo_Dispatch.Size(m)
}
func (m *Dispatch) XXX_DiscardUnknown() {
	xxx_messageInfo_Dispatch.DiscardUnknown(m)
}

var xxx_messageInfo_Dispatch proto.InternalMessageInfo

func (m *Dispatch) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *Dispatch) GetDomains() []string {
	if m != nil {
		return m.Domains
	}
	return nil
}

func (m *Dispatch) GetCluster() string {
	if m != nil {
		return m.Cluster
	}
	return ""
}

// DomainAlias regexp expression, which is alias for target domain
// default value is empty
// example:
// domainAliases:
//   - pattern: (?P<service>[^\.]+)\.(?P<namespace>[^\.]+)\.svc\.cluster\.local$
//     template:
//       - $namespace.$service.service.mailsaas
type DomainAlias struct {
	Pattern              string   `protobuf:"bytes,1,opt,name=pattern,proto3" json:"pattern,omitempty"`
	Templates            []string `protobuf:"bytes,2,rep,name=templates,proto3" json:"templates,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DomainAlias) Reset()         { *m = DomainAlias{} }
func (m *DomainAlias) String() string { return proto.CompactTextString(m) }
func (*DomainAlias) ProtoMessage()    {}
func (*DomainAlias) Descriptor() ([]byte, []int) {
	return fileDescriptor_8eebc4b237a55c9b, []int{2}
}
func (m *DomainAlias) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DomainAlias.Unmarshal(m, b)
}
func (m *DomainAlias) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DomainAlias.Marshal(b, m, deterministic)
}
func (m *DomainAlias) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DomainAlias.Merge(m, src)
}
func (m *DomainAlias) XXX_Size() int {
	return xxx_messageInfo_DomainAlias.Size(m)
}
func (m *DomainAlias) XXX_DiscardUnknown() {
	xxx_messageInfo_DomainAlias.DiscardUnknown(m)
}

var xxx_messageInfo_DomainAlias proto.InternalMessageInfo

func (m *DomainAlias) GetPattern() string {
	if m != nil {
		return m.Pattern
	}
	return ""
}

func (m *DomainAlias) GetTemplates() []string {
	if m != nil {
		return m.Templates
	}
	return nil
}

func init() {
	proto.RegisterType((*Fence)(nil), "slime.microservice.lazyload.config.Fence")
	proto.RegisterType((*Dispatch)(nil), "slime.microservice.lazyload.config.Dispatch")
	proto.RegisterType((*DomainAlias)(nil), "slime.microservice.lazyload.config.DomainAlias")
}

func init() { proto.RegisterFile("fence_module.proto", fileDescriptor_8eebc4b237a55c9b) }

var fileDescriptor_8eebc4b237a55c9b = []byte{
	// 383 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x52, 0x4d, 0x6b, 0xe3, 0x30,
	0x10, 0xc5, 0x71, 0x3e, 0x6c, 0x65, 0xf7, 0xb0, 0x3a, 0x89, 0x65, 0x0f, 0xc6, 0x27, 0xb3, 0x04,
	0x1b, 0x76, 0x7f, 0x41, 0x4b, 0x3f, 0x0e, 0x0d, 0xa5, 0xb8, 0xf4, 0xd2, 0x4b, 0x51, 0xec, 0x49,
	0x23, 0x90, 0x2d, 0x23, 0xc9, 0x2d, 0xe9, 0x4f, 0xed, 0xaf, 0x29, 0x1a, 0xc7, 0x71, 0x42, 0x0a,
	0xed, 0x4d, 0x33, 0x4f, 0xef, 0x31, 0xf3, 0xde, 0x10, 0xba, 0x86, 0xba, 0x80, 0xa7, 0x4a, 0x95,
	0xad, 0x84, 0xb4, 0xd1, 0xca, 0x2a, 0x1a, 0x1b, 0x29, 0x2a, 0x48, 0x2b, 0x51, 0x68, 0x65, 0x40,
	0xbf, 0x88, 0x02, 0x52, 0xc9, 0xdf, 0xb6, 0x52, 0xf1, 0x32, 0x2d, 0x54, 0xbd, 0x16, 0xcf, 0xf1,
	0xbb, 0x4f, 0x26, 0x57, 0x8e, 0x4a, 0x63, 0xf2, 0xe3, 0x55, 0xe9, 0x6a, 0xa3, 0x24, 0xdc, 0x29,
	0x6d, 0x99, 0x17, 0xf9, 0x49, 0x98, 0x1f, 0xf5, 0xe8, 0x1f, 0x12, 0xf2, 0xd6, 0x2a, 0x24, 0xb0,
	0x51, 0xe4, 0x25, 0x41, 0x3e, 0x34, 0x1c, 0x5a, 0xf3, 0x0a, 0x4c, 0xc3, 0x0b, 0x60, 0x3e, 0xd2,
	0x87, 0x06, 0x5d, 0x12, 0x52, 0x0a, 0xd3, 0x70, 0x5b, 0x6c, 0xc0, 0xb0, 0x71, 0xe4, 0x27, 0xf3,
	0x7f, 0x8b, 0xf4, 0xeb, 0x11, 0xd3, 0x8b, 0x1d, 0x2b, 0x3f, 0xe0, 0xd3, 0x07, 0xf2, 0xb3, 0x54,
	0x15, 0x17, 0xf5, 0x99, 0x14, 0xdc, 0x80, 0x61, 0x13, 0x14, 0xcc, 0xbe, 0x25, 0x38, 0x10, 0xf3,
	0x63, 0x15, 0x67, 0x42, 0x09, 0x6b, 0xde, 0x4a, 0xdb, 0xed, 0x38, 0xc5, 0x1d, 0x8f, 0x7a, 0xf4,
	0x37, 0x09, 0xdc, 0xce, 0x68, 0xd2, 0x0c, 0xf1, 0x7d, 0x4d, 0x53, 0x42, 0x0b, 0xd9, 0x1a, 0x0b,
	0xfa, 0xda, 0xdc, 0xee, 0xbd, 0x08, 0x22, 0x2f, 0x09, 0xf3, 0x4f, 0x10, 0xf7, 0x1f, 0x83, 0x5b,
	0xf2, 0x15, 0xc8, 0x1b, 0xd8, 0xe2, 0x1c, 0x2c, 0xec, 0xfe, 0x9f, 0x22, 0x74, 0x41, 0x7e, 0x41,
	0xcd, 0x57, 0x12, 0xee, 0x37, 0x4a, 0xdb, 0x6e, 0x11, 0x46, 0x70, 0x88, 0x53, 0x20, 0xce, 0x49,
	0xd0, 0x9b, 0x47, 0x29, 0x19, 0xbb, 0x2c, 0x98, 0x87, 0xda, 0xf8, 0xa6, 0x8c, 0xcc, 0xba, 0xf5,
	0x0d, 0x1b, 0x61, 0x5c, 0x7d, 0xe9, 0x90, 0xdd, 0xb4, 0xcc, 0x47, 0x42, 0x5f, 0xc6, 0x97, 0x64,
	0x7e, 0xe0, 0x9f, 0xfb, 0xd8, 0x70, 0x6b, 0x41, 0xd7, 0x3b, 0xe5, 0xbe, 0x74, 0xd7, 0x60, 0xa1,
	0x6a, 0x24, 0xb7, 0xd0, 0xcb, 0x0f, 0x8d, 0xf3, 0xc5, 0xe3, 0xdf, 0x2e, 0x29, 0xa1, 0x32, 0x7c,
	0x64, 0xdd, 0xe9, 0x9a, 0xac, 0x4f, 0x2b, 0xe3, 0x8d, 0xc8, 0xba, 0xc4, 0x56, 0x53, 0x3c, 0xe8,
	0xff, 0x1f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x69, 0x78, 0xf5, 0x33, 0xe6, 0x02, 0x00, 0x00,
}
