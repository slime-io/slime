package apis

import (
	"testing"

	"github.com/golang/protobuf/proto"
	config "slime.io/slime/framework/apis/config/v1alpha1"
	configtest "slime.io/slime/framework/apis/testdata/configv1alpha1"
)

func TestGeneral(t *testing.T) {
	g := &config.General{}
	gt := &configtest.General{
		Foo: "foo",
		Bar: &configtest.Bar{
			X: "y",
		},
	}
	bs, err := proto.Marshal(gt)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(bs, g); err != nil {
		t.Failed()
	}
	t.Log(g)

	gt2 := &configtest.General{}
	if err := gt2.XXX_Unmarshal(g.XXX_unrecognized); err != nil {
		t.Fatal(err)
	}
	t.Log(gt2)
	if !proto.Equal(gt, gt2) {
		t.Fatalf("gt != gt2")
	}

	gt3 := &configtest.General{}
	if err := proto.Unmarshal(g.XXX_unrecognized, gt3); err != nil {
		t.Fatal(err)
	}
	t.Log(gt3)
	if !proto.Equal(gt, gt3) {
		t.Fatalf("gt != gt3")
	}
}
