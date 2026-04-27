package main

import (
	"io"
	"os"
)

// captureStdout runs fn with os.Stdout pointed at an in-memory pipe
// and returns whatever fn wrote. Shared by every *_test.go in the
// package; defining it once prevents the multiply-defined collision
// that would otherwise occur between apply_test.go and list_test.go.
func captureStdout(fn func() error) (string, error) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	err := fn()
	w.Close()
	os.Stdout = old
	body, _ := io.ReadAll(r)
	return string(body), err
}
