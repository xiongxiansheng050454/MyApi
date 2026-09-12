package pricing

import "testing"

func TestComputeCost(t *testing.T) {
	// input 1000 (cached 400), output 500; pIn=30, pOut=60, pCached=15 per 1M
	got := ComputeCost(1000, 500, 400, 30, 60, 15)
	want := (float64(600)*30 + float64(400)*15 + float64(500)*60) / 1e6
	if got != want {
		t.Fatalf("ComputeCost=%v want %v", got, want)
	}
	if ComputeCost(0, 0, 0, 1, 1, 1) != 0 {
		t.Fatal("zero usage should be zero cost")
	}
}
