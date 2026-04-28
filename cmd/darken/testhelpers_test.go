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

// captureCombined runs fn with both stdout and stderr pointed at
// in-memory pipes and returns the concatenated output. Used by tests
// that exercise multi-step subcommands writing to a mix of streams
// (e.g. upgrade-init invokes init + doctor).
func captureCombined(fn func() error) (string, error) {
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	err := fn()
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	outBuf, _ := io.ReadAll(rOut)
	errBuf, _ := io.ReadAll(rErr)
	return string(outBuf) + string(errBuf), err
}
