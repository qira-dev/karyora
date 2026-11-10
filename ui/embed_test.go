package ui

import (
	"io"
	"testing"
)

func TestDist(t *testing.T) {
	dist, err := Dist()
	if err != nil {
		t.Fatalf("Dist() failed: %v", err)
	}
	f, err := dist.Open("index.html")
	if err != nil {
		t.Skipf("UI not built (run `npm run build` in ui/ before `go build`): %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("io.ReadAll failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("index.html is empty")
	}
}
