package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListInvokesScion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "scion"),
		[]byte(`#!/bin/sh
echo "NAME STATE TURNS"
echo "researcher running 5"
`), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	out, err := captureStdout(func() error { return runList(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "researcher") {
		t.Fatalf("list did not show researcher: %s", out)
	}
}
