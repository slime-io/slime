package wasm

import "strings"

type Getter interface {
	Get(wasmfile string) string
}

type LocalSource struct {
	Mount string
}

func (l *LocalSource) Get(wasmfile string) string {
	if strings.HasSuffix(wasmfile, "/") {
		return l.Mount + wasmfile
	} else {
		return l.Mount + "/" + wasmfile
	}
}
