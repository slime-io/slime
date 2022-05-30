// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: smart_limiter.proto

package v1alpha2

import (
	fmt "fmt"
	math "math"

	proto "github.com/gogo/protobuf/proto"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = proto.Marshal
	_ = fmt.Errorf
	_ = math.Inf
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type SmartLimiterSpec struct {
	// subset rate-limit,the key is subset name.
	Sets map[string]*SmartLimitDescriptors `protobuf:"bytes,1,rep,name=sets,proto3" json:"sets,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// rls service
	Rls                  string   `protobuf:"bytes,2,opt,name=rls,proto3" json:"rls,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SmartLimiterSpec) Reset()         { *m = SmartLimiterSpec{} }
func (m *SmartLimiterSpec) String() string { return proto.CompactTextString(m) }
func (*SmartLimiterSpec) ProtoMessage()    {}
func (*SmartLimiterSpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{0}
}

func (m *SmartLimiterSpec) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimiterSpec.Unmarshal(m, b)
}

func (m *SmartLimiterSpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimiterSpec.Marshal(b, m, deterministic)
}

func (m *SmartLimiterSpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimiterSpec.Merge(m, src)
}

func (m *SmartLimiterSpec) XXX_Size() int {
	return xxx_messageInfo_SmartLimiterSpec.Size(m)
}

func (m *SmartLimiterSpec) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimiterSpec.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimiterSpec proto.InternalMessageInfo

func (m *SmartLimiterSpec) GetSets() map[string]*SmartLimitDescriptors {
	if m != nil {
		return m.Sets
	}
	return nil
}

func (m *SmartLimiterSpec) GetRls() string {
	if m != nil {
		return m.Rls
	}
	return ""
}

type SmartLimiterStatus struct {
	RatelimitStatus      map[string]*SmartLimitDescriptors `protobuf:"bytes,1,rep,name=ratelimitStatus,proto3" json:"ratelimitStatus,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	MetricStatus         map[string]string                 `protobuf:"bytes,2,rep,name=metricStatus,proto3" json:"metricStatus,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	XXX_NoUnkeyedLiteral struct{}                          `json:"-"`
	XXX_unrecognized     []byte                            `json:"-"`
	XXX_sizecache        int32                             `json:"-"`
}

func (m *SmartLimiterStatus) Reset()         { *m = SmartLimiterStatus{} }
func (m *SmartLimiterStatus) String() string { return proto.CompactTextString(m) }
func (*SmartLimiterStatus) ProtoMessage()    {}
func (*SmartLimiterStatus) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{1}
}

func (m *SmartLimiterStatus) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimiterStatus.Unmarshal(m, b)
}

func (m *SmartLimiterStatus) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimiterStatus.Marshal(b, m, deterministic)
}

func (m *SmartLimiterStatus) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimiterStatus.Merge(m, src)
}

func (m *SmartLimiterStatus) XXX_Size() int {
	return xxx_messageInfo_SmartLimiterStatus.Size(m)
}

func (m *SmartLimiterStatus) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimiterStatus.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimiterStatus proto.InternalMessageInfo

func (m *SmartLimiterStatus) GetRatelimitStatus() map[string]*SmartLimitDescriptors {
	if m != nil {
		return m.RatelimitStatus
	}
	return nil
}

func (m *SmartLimiterStatus) GetMetricStatus() map[string]string {
	if m != nil {
		return m.MetricStatus
	}
	return nil
}

type SmartLimitDescriptor struct {
	Condition            string                                `protobuf:"bytes,1,opt,name=condition,proto3" json:"condition,omitempty"`
	Action               *SmartLimitDescriptor_Action          `protobuf:"bytes,2,opt,name=action,proto3" json:"action,omitempty"`
	Match                []*SmartLimitDescriptor_HeaderMatcher `protobuf:"bytes,3,rep,name=match,proto3" json:"match,omitempty"`
	Target               *SmartLimitDescriptor_Target          `protobuf:"bytes,4,opt,name=target,proto3" json:"target,omitempty"`
	CustomKey            string                                `protobuf:"bytes,5,opt,name=custom_key,json=customKey,proto3" json:"custom_key,omitempty"`
	CustomValue          string                                `protobuf:"bytes,6,opt,name=custom_value,json=customValue,proto3" json:"custom_value,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                              `json:"-"`
	XXX_unrecognized     []byte                                `json:"-"`
	XXX_sizecache        int32                                 `json:"-"`
}

func (m *SmartLimitDescriptor) Reset()         { *m = SmartLimitDescriptor{} }
func (m *SmartLimitDescriptor) String() string { return proto.CompactTextString(m) }
func (*SmartLimitDescriptor) ProtoMessage()    {}
func (*SmartLimitDescriptor) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{2}
}

func (m *SmartLimitDescriptor) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimitDescriptor.Unmarshal(m, b)
}

func (m *SmartLimitDescriptor) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimitDescriptor.Marshal(b, m, deterministic)
}

func (m *SmartLimitDescriptor) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimitDescriptor.Merge(m, src)
}

func (m *SmartLimitDescriptor) XXX_Size() int {
	return xxx_messageInfo_SmartLimitDescriptor.Size(m)
}

func (m *SmartLimitDescriptor) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimitDescriptor.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimitDescriptor proto.InternalMessageInfo

func (m *SmartLimitDescriptor) GetCondition() string {
	if m != nil {
		return m.Condition
	}
	return ""
}

func (m *SmartLimitDescriptor) GetAction() *SmartLimitDescriptor_Action {
	if m != nil {
		return m.Action
	}
	return nil
}

func (m *SmartLimitDescriptor) GetMatch() []*SmartLimitDescriptor_HeaderMatcher {
	if m != nil {
		return m.Match
	}
	return nil
}

func (m *SmartLimitDescriptor) GetTarget() *SmartLimitDescriptor_Target {
	if m != nil {
		return m.Target
	}
	return nil
}

func (m *SmartLimitDescriptor) GetCustomKey() string {
	if m != nil {
		return m.CustomKey
	}
	return ""
}

func (m *SmartLimitDescriptor) GetCustomValue() string {
	if m != nil {
		return m.CustomValue
	}
	return ""
}

type SmartLimitDescriptor_HeaderMatcher struct {
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// If specified, this regex string is a regular expression rule which implies the entire request
	// header value must match the regex. The rule will not match if only a subsequence of the
	// request header value matches the regex.
	RegexMatch string `protobuf:"bytes,2,opt,name=regex_match,json=regexMatch,proto3" json:"regex_match,omitempty"`
	// If specified, header match will be performed based on the value of the header.
	ExactMatch string `protobuf:"bytes,3,opt,name=exact_match,json=exactMatch,proto3" json:"exact_match,omitempty"`
	// * The prefix *abcd* matches the value *abcdxyz*, but not for *abcxyz*.
	PrefixMatch string `protobuf:"bytes,4,opt,name=prefix_match,json=prefixMatch,proto3" json:"prefix_match,omitempty"`
	// * The suffix *abcd* matches the value *xyzabcd*, but not for *xyzbcd*.
	SuffixMatch string `protobuf:"bytes,5,opt,name=suffix_match,json=suffixMatch,proto3" json:"suffix_match,omitempty"`
	// If specified as true, header match will be performed based on whether the header is in the
	// request. If specified as false, header match will be performed based on whether the header is absent.
	PresentMatch bool `protobuf:"varint,6,opt,name=present_match,json=presentMatch,proto3" json:"present_match,omitempty"`
	// If specified, the match result will be inverted before checking. Defaults to false.
	// * The regex ``\d{3}`` does not match the value *1234*, so it will match when inverted.
	InvertMatch bool `protobuf:"varint,7,opt,name=invert_match,json=invertMatch,proto3" json:"invert_match,omitempty"`
	// if specified, the exact match the value ""
	IsExactMatchEmpty    bool     `protobuf:"varint,8,opt,name=is_exact_match_empty,json=isExactMatchEmpty,proto3" json:"is_exact_match_empty,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SmartLimitDescriptor_HeaderMatcher) Reset()         { *m = SmartLimitDescriptor_HeaderMatcher{} }
func (m *SmartLimitDescriptor_HeaderMatcher) String() string { return proto.CompactTextString(m) }
func (*SmartLimitDescriptor_HeaderMatcher) ProtoMessage()    {}
func (*SmartLimitDescriptor_HeaderMatcher) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{2, 0}
}

func (m *SmartLimitDescriptor_HeaderMatcher) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimitDescriptor_HeaderMatcher.Unmarshal(m, b)
}

func (m *SmartLimitDescriptor_HeaderMatcher) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimitDescriptor_HeaderMatcher.Marshal(b, m, deterministic)
}

func (m *SmartLimitDescriptor_HeaderMatcher) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimitDescriptor_HeaderMatcher.Merge(m, src)
}

func (m *SmartLimitDescriptor_HeaderMatcher) XXX_Size() int {
	return xxx_messageInfo_SmartLimitDescriptor_HeaderMatcher.Size(m)
}

func (m *SmartLimitDescriptor_HeaderMatcher) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimitDescriptor_HeaderMatcher.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimitDescriptor_HeaderMatcher proto.InternalMessageInfo

func (m *SmartLimitDescriptor_HeaderMatcher) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetRegexMatch() string {
	if m != nil {
		return m.RegexMatch
	}
	return ""
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetExactMatch() string {
	if m != nil {
		return m.ExactMatch
	}
	return ""
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetPrefixMatch() string {
	if m != nil {
		return m.PrefixMatch
	}
	return ""
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetSuffixMatch() string {
	if m != nil {
		return m.SuffixMatch
	}
	return ""
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetPresentMatch() bool {
	if m != nil {
		return m.PresentMatch
	}
	return false
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetInvertMatch() bool {
	if m != nil {
		return m.InvertMatch
	}
	return false
}

func (m *SmartLimitDescriptor_HeaderMatcher) GetIsExactMatchEmpty() bool {
	if m != nil {
		return m.IsExactMatchEmpty
	}
	return false
}

type SmartLimitDescriptor_Action struct {
	Quota                string    `protobuf:"bytes,1,opt,name=quota,proto3" json:"quota,omitempty"`
	FillInterval         *Duration `protobuf:"bytes,2,opt,name=fill_interval,json=fillInterval,proto3" json:"fill_interval,omitempty"`
	Strategy             string    `protobuf:"bytes,3,opt,name=strategy,proto3" json:"strategy,omitempty"`
	XXX_NoUnkeyedLiteral struct{}  `json:"-"`
	XXX_unrecognized     []byte    `json:"-"`
	XXX_sizecache        int32     `json:"-"`
}

func (m *SmartLimitDescriptor_Action) Reset()         { *m = SmartLimitDescriptor_Action{} }
func (m *SmartLimitDescriptor_Action) String() string { return proto.CompactTextString(m) }
func (*SmartLimitDescriptor_Action) ProtoMessage()    {}
func (*SmartLimitDescriptor_Action) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{2, 1}
}

func (m *SmartLimitDescriptor_Action) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimitDescriptor_Action.Unmarshal(m, b)
}

func (m *SmartLimitDescriptor_Action) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimitDescriptor_Action.Marshal(b, m, deterministic)
}

func (m *SmartLimitDescriptor_Action) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimitDescriptor_Action.Merge(m, src)
}

func (m *SmartLimitDescriptor_Action) XXX_Size() int {
	return xxx_messageInfo_SmartLimitDescriptor_Action.Size(m)
}

func (m *SmartLimitDescriptor_Action) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimitDescriptor_Action.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimitDescriptor_Action proto.InternalMessageInfo

func (m *SmartLimitDescriptor_Action) GetQuota() string {
	if m != nil {
		return m.Quota
	}
	return ""
}

func (m *SmartLimitDescriptor_Action) GetFillInterval() *Duration {
	if m != nil {
		return m.FillInterval
	}
	return nil
}

func (m *SmartLimitDescriptor_Action) GetStrategy() string {
	if m != nil {
		return m.Strategy
	}
	return ""
}

type SmartLimitDescriptor_Target struct {
	Direction            string   `protobuf:"bytes,1,opt,name=direction,proto3" json:"direction,omitempty"`
	Port                 int32    `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
	Route                []string `protobuf:"bytes,3,rep,name=route,proto3" json:"route,omitempty"`
	Host                 []string `protobuf:"bytes,4,rep,name=host,proto3" json:"host,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SmartLimitDescriptor_Target) Reset()         { *m = SmartLimitDescriptor_Target{} }
func (m *SmartLimitDescriptor_Target) String() string { return proto.CompactTextString(m) }
func (*SmartLimitDescriptor_Target) ProtoMessage()    {}
func (*SmartLimitDescriptor_Target) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{2, 2}
}

func (m *SmartLimitDescriptor_Target) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimitDescriptor_Target.Unmarshal(m, b)
}

func (m *SmartLimitDescriptor_Target) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimitDescriptor_Target.Marshal(b, m, deterministic)
}

func (m *SmartLimitDescriptor_Target) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimitDescriptor_Target.Merge(m, src)
}

func (m *SmartLimitDescriptor_Target) XXX_Size() int {
	return xxx_messageInfo_SmartLimitDescriptor_Target.Size(m)
}

func (m *SmartLimitDescriptor_Target) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimitDescriptor_Target.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimitDescriptor_Target proto.InternalMessageInfo

func (m *SmartLimitDescriptor_Target) GetDirection() string {
	if m != nil {
		return m.Direction
	}
	return ""
}

func (m *SmartLimitDescriptor_Target) GetPort() int32 {
	if m != nil {
		return m.Port
	}
	return 0
}

func (m *SmartLimitDescriptor_Target) GetRoute() []string {
	if m != nil {
		return m.Route
	}
	return nil
}

func (m *SmartLimitDescriptor_Target) GetHost() []string {
	if m != nil {
		return m.Host
	}
	return nil
}

type SmartLimitDescriptors struct {
	// Description of current rate-limit
	Descriptor_          []*SmartLimitDescriptor `protobuf:"bytes,1,rep,name=descriptor,proto3" json:"descriptor,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                `json:"-"`
	XXX_unrecognized     []byte                  `json:"-"`
	XXX_sizecache        int32                   `json:"-"`
}

func (m *SmartLimitDescriptors) Reset()         { *m = SmartLimitDescriptors{} }
func (m *SmartLimitDescriptors) String() string { return proto.CompactTextString(m) }
func (*SmartLimitDescriptors) ProtoMessage()    {}
func (*SmartLimitDescriptors) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{3}
}

func (m *SmartLimitDescriptors) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SmartLimitDescriptors.Unmarshal(m, b)
}

func (m *SmartLimitDescriptors) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SmartLimitDescriptors.Marshal(b, m, deterministic)
}

func (m *SmartLimitDescriptors) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SmartLimitDescriptors.Merge(m, src)
}

func (m *SmartLimitDescriptors) XXX_Size() int {
	return xxx_messageInfo_SmartLimitDescriptors.Size(m)
}

func (m *SmartLimitDescriptors) XXX_DiscardUnknown() {
	xxx_messageInfo_SmartLimitDescriptors.DiscardUnknown(m)
}

var xxx_messageInfo_SmartLimitDescriptors proto.InternalMessageInfo

func (m *SmartLimitDescriptors) GetDescriptor_() []*SmartLimitDescriptor {
	if m != nil {
		return m.Descriptor_
	}
	return nil
}

type Duration struct {
	// Signed seconds of the span of time. Must be from -315,576,000,000
	// to +315,576,000,000 inclusive. Note: these bounds are computed from:
	// 60 sec/min * 60 min/hr * 24 hr/day * 365.25 days/year * 10000 years
	Seconds int64 `protobuf:"varint,1,opt,name=seconds,proto3" json:"seconds,omitempty"`
	// Signed fractions of a second at nanosecond resolution of the span
	// of time. Durations less than one second are represented with a 0
	// `seconds` field and a positive or negative `nanos` field. For durations
	// of one second or more, a non-zero value for the `nanos` field must be
	// of the same sign as the `seconds` field. Must be from -999,999,999
	// to +999,999,999 inclusive.
	Nanos                int32    `protobuf:"varint,2,opt,name=nanos,proto3" json:"nanos,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Duration) Reset()         { *m = Duration{} }
func (m *Duration) String() string { return proto.CompactTextString(m) }
func (*Duration) ProtoMessage()    {}
func (*Duration) Descriptor() ([]byte, []int) {
	return fileDescriptor_452a0625a4f6276b, []int{4}
}

func (m *Duration) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Duration.Unmarshal(m, b)
}

func (m *Duration) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Duration.Marshal(b, m, deterministic)
}

func (m *Duration) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Duration.Merge(m, src)
}

func (m *Duration) XXX_Size() int {
	return xxx_messageInfo_Duration.Size(m)
}

func (m *Duration) XXX_DiscardUnknown() {
	xxx_messageInfo_Duration.DiscardUnknown(m)
}

var xxx_messageInfo_Duration proto.InternalMessageInfo

func (m *Duration) GetSeconds() int64 {
	if m != nil {
		return m.Seconds
	}
	return 0
}

func (m *Duration) GetNanos() int32 {
	if m != nil {
		return m.Nanos
	}
	return 0
}

func init() {
	proto.RegisterType((*SmartLimiterSpec)(nil), "slime.microservice.limiter.v1alpha2.SmartLimiterSpec")
	proto.RegisterMapType((map[string]*SmartLimitDescriptors)(nil), "slime.microservice.limiter.v1alpha2.SmartLimiterSpec.SetsEntry")
	proto.RegisterType((*SmartLimiterStatus)(nil), "slime.microservice.limiter.v1alpha2.SmartLimiterStatus")
	proto.RegisterMapType((map[string]string)(nil), "slime.microservice.limiter.v1alpha2.SmartLimiterStatus.MetricStatusEntry")
	proto.RegisterMapType((map[string]*SmartLimitDescriptors)(nil), "slime.microservice.limiter.v1alpha2.SmartLimiterStatus.RatelimitStatusEntry")
	proto.RegisterType((*SmartLimitDescriptor)(nil), "slime.microservice.limiter.v1alpha2.SmartLimitDescriptor")
	proto.RegisterType((*SmartLimitDescriptor_HeaderMatcher)(nil), "slime.microservice.limiter.v1alpha2.SmartLimitDescriptor.HeaderMatcher")
	proto.RegisterType((*SmartLimitDescriptor_Action)(nil), "slime.microservice.limiter.v1alpha2.SmartLimitDescriptor.Action")
	proto.RegisterType((*SmartLimitDescriptor_Target)(nil), "slime.microservice.limiter.v1alpha2.SmartLimitDescriptor.Target")
	proto.RegisterType((*SmartLimitDescriptors)(nil), "slime.microservice.limiter.v1alpha2.SmartLimitDescriptors")
	proto.RegisterType((*Duration)(nil), "slime.microservice.limiter.v1alpha2.Duration")
}

func init() { proto.RegisterFile("smart_limiter.proto", fileDescriptor_452a0625a4f6276b) }

var fileDescriptor_452a0625a4f6276b = []byte{
	// 707 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xbc, 0x55, 0xdd, 0x4e, 0x13, 0x4d,
	0x18, 0x4e, 0x7f, 0x69, 0xdf, 0x96, 0x7c, 0x30, 0x5f, 0x4d, 0x9a, 0x8d, 0x46, 0x28, 0x27, 0x9c,
	0xb0, 0x0d, 0x70, 0xa2, 0x9c, 0xa0, 0x06, 0xa2, 0x44, 0x48, 0xcc, 0xd6, 0x18, 0x35, 0x31, 0x9b,
	0x71, 0xfb, 0x02, 0x13, 0x76, 0x77, 0xd6, 0x99, 0xd9, 0x86, 0x9e, 0x78, 0x09, 0x5e, 0x85, 0x89,
	0x37, 0xe1, 0x35, 0x79, 0x0d, 0x66, 0x7e, 0x76, 0x69, 0xb1, 0x07, 0x50, 0x13, 0x4f, 0x9a, 0x99,
	0x67, 0x9e, 0x79, 0xde, 0xe7, 0xfd, 0xd9, 0x29, 0xfc, 0x2f, 0x13, 0x2a, 0x54, 0x18, 0xb3, 0x84,
	0x29, 0x14, 0x7e, 0x26, 0xb8, 0xe2, 0x64, 0x4b, 0xc6, 0x2c, 0x41, 0x3f, 0x61, 0x91, 0xe0, 0x12,
	0xc5, 0x84, 0x45, 0xe8, 0x17, 0x8c, 0xc9, 0x2e, 0x8d, 0xb3, 0x4b, 0xba, 0x37, 0xf8, 0x55, 0x81,
	0xb5, 0x91, 0xbe, 0x7c, 0x6a, 0x4f, 0x46, 0x19, 0x46, 0x64, 0x04, 0x75, 0x89, 0x4a, 0xf6, 0x2b,
	0x1b, 0xb5, 0xed, 0xce, 0xde, 0xa1, 0x7f, 0x07, 0x21, 0xff, 0xb6, 0x88, 0x3f, 0x42, 0x25, 0x8f,
	0x53, 0x25, 0xa6, 0x81, 0x11, 0x23, 0x6b, 0x50, 0x13, 0xb1, 0xec, 0x57, 0x37, 0x2a, 0xdb, 0xed,
	0x40, 0x2f, 0x3d, 0x09, 0xed, 0x92, 0xa4, 0x8f, 0xaf, 0x70, 0xda, 0xaf, 0xd8, 0xe3, 0x2b, 0x9c,
	0x92, 0x37, 0xd0, 0x98, 0xd0, 0x38, 0x47, 0x73, 0xa5, 0xb3, 0x77, 0x70, 0x4f, 0x1b, 0x47, 0x28,
	0x23, 0xc1, 0x32, 0xc5, 0x85, 0x0c, 0xac, 0xd0, 0x41, 0xf5, 0x49, 0x65, 0xf0, 0xb3, 0x06, 0x64,
	0xce, 0xab, 0xa2, 0x2a, 0x97, 0x64, 0x02, 0xff, 0x09, 0xaa, 0xd0, 0xe8, 0x59, 0xc8, 0x65, 0x7f,
	0x7a, 0xff, 0xec, 0xcd, 0x75, 0x3f, 0x98, 0x97, 0xb3, 0xa5, 0xb8, 0x1d, 0x84, 0x24, 0xd0, 0x4d,
	0x50, 0x09, 0x16, 0xb9, 0xa0, 0x55, 0x13, 0xf4, 0x64, 0xd9, 0xa0, 0x67, 0x33, 0x5a, 0x36, 0xe2,
	0x9c, 0xbc, 0xf7, 0x15, 0x7a, 0x8b, 0x7c, 0xfd, 0xab, 0xea, 0x7b, 0x87, 0xb0, 0xfe, 0x87, 0xc5,
	0x05, 0xc1, 0x7b, 0xb3, 0xc1, 0xdb, 0xb3, 0xed, 0xfb, 0xb1, 0x02, 0xbd, 0x45, 0x51, 0xc8, 0x43,
	0x68, 0x47, 0x3c, 0x1d, 0x33, 0xc5, 0x78, 0xea, 0xa4, 0x6e, 0x00, 0xf2, 0x1e, 0x9a, 0x34, 0x32,
	0x47, 0x36, 0x9d, 0x67, 0x4b, 0xa7, 0xe3, 0x3f, 0x37, 0x3a, 0x81, 0xd3, 0x23, 0x9f, 0xa0, 0x91,
	0x50, 0x15, 0x5d, 0xf6, 0x6b, 0xa6, 0x73, 0x2f, 0x97, 0x17, 0x7e, 0x85, 0x74, 0x8c, 0xe2, 0x4c,
	0x8b, 0xa1, 0x08, 0xac, 0xaa, 0x36, 0xae, 0xa8, 0xb8, 0x40, 0xd5, 0xaf, 0xff, 0xad, 0xf1, 0xb7,
	0x46, 0x27, 0x70, 0x7a, 0xe4, 0x11, 0x40, 0x94, 0x4b, 0xc5, 0x93, 0x50, 0x17, 0xbf, 0xe1, 0x2a,
	0x66, 0x90, 0xd7, 0x38, 0x25, 0x9b, 0xd0, 0x75, 0xc7, 0xb6, 0x13, 0x4d, 0x43, 0xe8, 0x58, 0xec,
	0x9d, 0x86, 0xbc, 0xef, 0x55, 0x58, 0x9d, 0x33, 0x4d, 0x08, 0xd4, 0x53, 0x9a, 0xa0, 0xab, 0xbf,
	0x59, 0x93, 0xc7, 0xd0, 0x11, 0x78, 0x81, 0xd7, 0xa1, 0x2d, 0x93, 0xed, 0x28, 0x18, 0xc8, 0x5c,
	0xd3, 0x04, 0xbc, 0xa6, 0x91, 0x0a, 0x8b, 0x3a, 0x1a, 0x82, 0x81, 0x2c, 0x61, 0x13, 0xba, 0x99,
	0xc0, 0x73, 0x56, 0x48, 0xd4, 0xad, 0x15, 0x8b, 0x95, 0x14, 0x99, 0x9f, 0xdf, 0x50, 0x6c, 0x3a,
	0x1d, 0x8b, 0x59, 0xca, 0x16, 0xac, 0x66, 0x02, 0x25, 0xa6, 0x45, 0x20, 0x9d, 0x51, 0x2b, 0xe8,
	0x3a, 0xb0, 0xd4, 0x61, 0xe9, 0x04, 0x45, 0xc1, 0x59, 0x31, 0x9c, 0x8e, 0xc5, 0x2c, 0x65, 0x08,
	0x3d, 0x26, 0xc3, 0x19, 0xc7, 0x21, 0x26, 0x99, 0x9a, 0xf6, 0x5b, 0x86, 0xba, 0xce, 0xe4, 0x71,
	0xe9, 0xfc, 0x58, 0x1f, 0x78, 0xdf, 0x2a, 0xd0, 0xb4, 0x43, 0xa3, 0xe7, 0xfa, 0x4b, 0xce, 0x15,
	0x75, 0x05, 0xb2, 0x1b, 0x12, 0xc0, 0xea, 0x39, 0x8b, 0xe3, 0x90, 0xa5, 0x0a, 0xc5, 0x84, 0xc6,
	0x6e, 0x46, 0x77, 0xee, 0xd4, 0xea, 0xa3, 0x5c, 0x50, 0x33, 0x90, 0x5d, 0xad, 0x71, 0xe2, 0x24,
	0x88, 0x07, 0x2d, 0xa9, 0xf4, 0x63, 0x73, 0x31, 0x75, 0x15, 0x2d, 0xf7, 0xde, 0x18, 0x9a, 0x76,
	0x16, 0xf4, 0x47, 0x33, 0x66, 0x02, 0xa3, 0xd9, 0x8f, 0xa6, 0x04, 0x74, 0x37, 0x33, 0x2e, 0x94,
	0xb1, 0xd3, 0x08, 0xcc, 0x5a, 0x67, 0x20, 0x78, 0xae, 0xd0, 0x8c, 0x7b, 0x3b, 0xb0, 0x1b, 0xcd,
	0xbc, 0xe4, 0x52, 0xcf, 0xa8, 0x06, 0xcd, 0x7a, 0x20, 0xe0, 0xc1, 0xc2, 0xe7, 0x80, 0x7c, 0x00,
	0x18, 0x97, 0x5b, 0xf7, 0xca, 0x3e, 0x5d, 0x7a, 0xac, 0x83, 0x19, 0xb1, 0xc1, 0x01, 0xb4, 0x8a,
	0x7a, 0x90, 0x3e, 0xac, 0x48, 0xd4, 0x2f, 0x80, 0x34, 0x99, 0xd5, 0x82, 0x62, 0xab, 0x73, 0x48,
	0x69, 0xca, 0xa5, 0x4b, 0xcc, 0x6e, 0x5e, 0xec, 0x7f, 0xdc, 0xb5, 0x1e, 0x18, 0x1f, 0x9a, 0x85,
	0xfd, 0xdd, 0x49, 0xf8, 0x38, 0x8f, 0x51, 0x0e, 0x9d, 0x9b, 0x21, 0xcd, 0xd8, 0xb0, 0x70, 0xf4,
	0xb9, 0x69, 0xfe, 0x6a, 0xf7, 0x7f, 0x07, 0x00, 0x00, 0xff, 0xff, 0xf9, 0x52, 0x72, 0xe3, 0x81,
	0x07, 0x00, 0x00,
}
