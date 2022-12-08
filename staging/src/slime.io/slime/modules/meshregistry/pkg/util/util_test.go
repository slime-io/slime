package util

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDuration_MarshalJSON(t *testing.T) {
	type tp struct {
		D Duration
	}

	v := tp{D: Duration(2 * time.Second)}
	bs, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	var v1 tp
	if err = json.Unmarshal(bs, &v1); err != nil {
		t.Fatalf("%s %v", string(bs), err)
	}

	if v != v1 {
		t.Fatalf("%+v %+v", v, v1)
	}
}
