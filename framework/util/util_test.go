package util

import (
	"reflect"
	"testing"
)

func TestMapToMapInterface(t *testing.T) {
	ssMap := map[string]string{
		"a.b.c0": "d0",
		"a.b.c1": "d1",
		"c2":     "d2",
	}
	iMap := MapToMapInterface(ssMap)
	expected := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c0": "d0",
				"c1": "d1",
			},
		},
		"c2": "d2",
	}
	if !reflect.DeepEqual(iMap, expected) {
		t.Fatalf("expected: %v, but got %v", expected, iMap)
	}
}
