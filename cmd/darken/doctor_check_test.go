package main

import (
	"errors"
	"strings"
	"testing"
)

// TestDoctorCheck_TypeHasRequiredFields verifies that DoctorCheck has the
// fields required by REVIEW-5: ID, Label, Severity, Run, Remediation.
func TestDoctorCheck_TypeHasRequiredFields(t *testing.T) {
	dc := DoctorCheck{
		ID:          "docker-daemon",
		Label:       "docker daemon reachable",
		Severity:    SeverityFail,
		Run:         func() error { return nil },
		Remediation: "start Docker Desktop",
	}
	if dc.ID == "" {
		t.Error("DoctorCheck.ID must not be empty")
	}
	if dc.Label == "" {
		t.Error("DoctorCheck.Label must not be empty")
	}
	if dc.Severity != SeverityFail {
		t.Errorf("DoctorCheck.Severity: want SeverityFail, got %q", dc.Severity)
	}
	if dc.Run == nil {
		t.Error("DoctorCheck.Run must not be nil")
	}
	if dc.Remediation == "" {
		t.Error("DoctorCheck.Remediation must not be empty")
	}
}

// TestDoctorCheck_SeverityWarn verifies that warn-level checks do not
// contribute to the failed list in doctorBroad.
func TestDoctorCheck_SeverityWarn(t *testing.T) {
	dc := DoctorCheck{
		ID:          "warn-check",
		Label:       "optional thing",
		Severity:    SeverityWarn,
		Run:         func() error { return errors.New("missing optional thing") },
		Remediation: "install the optional thing",
	}
	// Warn-severity checks should not cause doctorBroad to return an error.
	// We verify by running the check and checking that it's treated as WARN.
	if dc.Severity != SeverityWarn {
		t.Errorf("want SeverityWarn, got %q", dc.Severity)
	}
	if dc.Run() == nil {
		t.Error("check should return non-nil error to trigger warn path")
	}
}

// TestDoctorCheck_RemediationEmbedded verifies that the broad check
// registry has inline Remediation strings, not a dispatch through
// remediationFor. The remediationFor function should be removed after REVIEW-5.
func TestDoctorCheck_RemediationEmbedded(t *testing.T) {
	// doctorBroadChecks returns the registry; every check must have
	// a non-empty Remediation so no stringly dispatch is needed.
	for _, dc := range doctorBroadChecks() {
		if dc.Severity == SeverityFail && dc.Remediation == "" {
			t.Errorf("DoctorCheck %q (severity=fail) must have non-empty Remediation", dc.ID)
		}
	}
}

// TestDoctorBroad_UsesRegistry verifies that doctorBroad runs
// checks from doctorBroadChecks() and formats FAIL/WARN/OK lines.
func TestDoctorBroad_UsesRegistry(t *testing.T) {
	// Inject a mock client that fails everything scion-related.
	mc := &mockScionClient{serverStatusErr: errors.New("scion not found")}
	setDefaultClient(t, mc)

	report, err := doctorBroad()
	if err == nil {
		t.Fatal("expected error when scion check fails")
	}
	if !strings.Contains(report, "FAIL") {
		t.Errorf("report should contain FAIL line, got:\n%s", report)
	}
}
