package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubScionServerStatus plants a fake `scion` binary that returns a
// known status output for `scion server status` calls.
func stubScionServerStatus(t *testing.T, statusBody string) {
	t.Helper()
	stubDir := t.TempDir()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"server\" ] && [ \"$2\" = \"status\" ]; then\n" +
		"  cat <<'EOF'\n" + statusBody + "\nEOF\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, "scion"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
}

// stubOpener plants a fake `open` (macOS) or `xdg-open` (Linux) that
// records the URL it was called with to a log file.
func stubOpener(t *testing.T, openerName string) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "opened.log")
	body := "#!/bin/sh\necho \"$1\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(stubDir, openerName), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend stub dir to PATH (existing scion stub still wins for scion calls).
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

func TestDashboard_DefaultURLOpened(t *testing.T) {
	statusOut := `Scion Server Status
  Daemon:        running (PID: 12345)
  Log file:      /Users/dmestas/.scion/server.log
  PID file:      /Users/dmestas/.scion/server.pid

Components:
  Hub API:         running
`
	stubScionServerStatus(t, statusOut)
	// runDashboard picks the opener by GOOS: `open` on macOS, `xdg-open`
	// on Linux. Mirror that here so the test exercises the same path
	// the production code takes on the runner OS.
	openerName := "open"
	if runtime.GOOS == "linux" {
		openerName = "xdg-open"
	}
	logPath := stubOpener(t, openerName)

	if err := runDashboard(nil); err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}

	body, _ := os.ReadFile(logPath)
	if !strings.Contains(string(body), "http://localhost:8080") {
		t.Fatalf("expected dashboard to open http://localhost:8080, got: %q", body)
	}
}

func TestDashboard_RejectsArgs(t *testing.T) {
	stubScionServerStatus(t, "")
	if err := runDashboard([]string{"foo"}); err == nil {
		t.Fatal("expected error when args provided")
	}
}

func TestDashboard_FailsWhenServerDown(t *testing.T) {
	// scion server status exits non-zero when daemon isn't running.
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "scion"),
		[]byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := runDashboard(nil)
	if err == nil {
		t.Fatal("expected error when scion server is down")
	}
	if !strings.Contains(err.Error(), "scion server") {
		t.Fatalf("error should mention scion server: %v", err)
	}
}
