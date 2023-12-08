package util

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

type AnyMessage struct {
	RawJson []byte
}

func (a *AnyMessage) Reset() {
}

func (a *AnyMessage) String() string {
	return ""
}

func (a *AnyMessage) ProtoMessage() {
}

func (a *AnyMessage) ProtoReflect() protoreflect.Message {
	return nil
}
