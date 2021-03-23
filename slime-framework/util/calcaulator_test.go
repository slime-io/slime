package util

import (
	"testing"
)

func TestCalcaulator(t *testing.T) {
	a1, _ := Calculate("11+1")
	a2, _ := Calculate("16/2")
	a3, _ := Calculate("15-23")
	a4, _ := Calculate("35*23")
	a5, _ := Calculate("4+5*6+((7+8)/6)-8")
	a6, _ := Calculate("4+5*6+((7+8)/6)>10")
	a7, _ := Calculate("4+5*6+((7+8)/6)>10|16/2>10")
	a8, _ := Calculate("4+5*6+((7+8)/6)>10&16/2>10")
	a9, _ := Calculate("10")

	a10, _ := CalculateTemplate("{{._ratelimit}}/{{.v1.epNum}}", map[string]interface{}{
		"_ratelimit": "100",
		"v1": map[string]interface{}{
			"epNum": "2",
		},
	})

	if a1 != 11+1 {
		t.Fatalf("test failed, expected: %d, actual: %d", 12, a1)
	}
	if a2 != 16/2 {
		t.Fatalf("test failed, expected: %d, actual: %d", 16/2, a2)
	}
	if a3 != 15-23 {
		t.Fatalf("test failed, expected: %d, actual: %d", 15-23, a3)
	}
	if a4 != 35*23 {
		t.Fatalf("test failed, expected: %d, actual: %d", 35*23, a4)
	}
	if a5 != 29 {
		t.Fatalf("test failed, expected: %d, actual: %d", 29, a5)
	}
	if a6 != 1 {
		t.Fatalf("test failed, expected: %d, actual: %d", 1, a5)
	}
	if a7 != 1 {
		t.Fatalf("test failed, expected: %d, actual: %d", 1, a5)
	}
	if a8 != 0 {
		t.Fatalf("test failed, expected: %d, actual: %d", 0, a5)
	}
	if a9 != 10 {
		t.Fatalf("test failed, expected: %d, actual: %d", 10, a9)
	}
	if a10 != 50 {
		t.Fatalf("test failed, expected: %d, actual: %d", 50, a10)
	}
}

func TestCalculateTemplate(t *testing.T) {
	tmap := map[string]string{
		"v1.cpu.sum": "8845.137062837",
		"v1.pod":     "1",
		"v2.cpu.sum": "10598.478665913",
		"v2.pod":     "1",
		"v3.cpu.sum": "10636.432688953",
		"v3.pod":     "1",
	}
	imap := MapToMapInterface(tmap)
	actual, err := CalculateTemplateBool("{{.v1.cpu.sum}}>8000", imap)
	if err != nil {
		t.Fatalf("test failed, got error:%s", err.Error())
	}
	if !actual {
		t.Fatalf("test failed, excepted: true, but got false")
	}
}
