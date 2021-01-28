package util

import "testing"

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

	a10, _ := CalculateTemplate("{{.ratelimit}}/{{.v1.epNum}}", map[string]interface{}{
		"ratelimit": "100",
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
	if a5 != 4+5*6+((7+8)/6-8) {
		t.Fatalf("test failed, expected: %d, actual: %d", 4+5*6+((7+8)/6), a5)
	}
	if a6 != 1 {
		t.Fatalf("test failed, expected: %d, actual: %d", 4+5*6+((7+8)/6), a5)
	}
	if a7 != 1 {
		t.Fatalf("test failed, expected: %d, actual: %d", 4+5*6+((7+8)/6), a5)
	}
	if a8 != 0 {
		t.Fatalf("test failed, expected: %d, actual: %d", 4+5*6+((7+8)/6), a5)
	}
	if a9 != 10 {
		t.Fatalf("test failed, expected: %d, actual: %d", 10, a9)
	}
	if a10 != 50 {
		t.Fatalf("test failed, expected: %d, actual: %d", 50, a10)
	}
}
