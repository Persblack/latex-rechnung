package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestClassicSingleRateVat verifies the classic single-rate VAT rendering for
// the Selim invoice: VAT is summarised once in the main table (Corff-native),
// with no per-item "(19% MwSt.)" labels, no separate breakdown table, and no
// duplicate project-level VAT row. Skipped without the LaTeX/poppler toolchain.
func TestClassicSingleRateVat(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	if err := loadProfiles("profiles"); err != nil {
		t.Fatalf("loadProfiles: %v", err)
	}
	if err := loadBankAccounts("bankaccounts.json"); err != nil {
		t.Fatalf("loadBankAccounts: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("invoices", "so-2026-001.json"))
	if err != nil {
		t.Fatalf("read invoice: %v", err)
	}
	var req InvoiceRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	pdf, cleanup, err := buildDocument(req, profiles[req.ProfileKey], docConfigs["invoice"], req.Design, "invoice")
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	defer cleanup()
	out, err := exec.Command("pdftotext", pdf, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	s := string(out)

	mustContain := []string{"zzgl. 19", "Gesamtsumme", "67 Std.", "14.910,00", "17.742,90"}
	for _, want := range mustContain {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in classic invoice text", want)
		}
	}
	mustNotContain := []string{"(19% MwSt.)", "MWSt. (19%)", "Aufschlüsselung"}
	for _, bad := range mustNotContain {
		if strings.Contains(s, bad) {
			t.Errorf("did not expect %q in classic invoice text", bad)
		}
	}
	// VAT amount must appear exactly once (no project-level duplicate).
	if n := strings.Count(s, "2.832,90"); n != 1 {
		t.Errorf("VAT amount 2.832,90 should appear once, got %d", n)
	}
}
