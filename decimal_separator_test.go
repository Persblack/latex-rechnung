package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDecimalSeparator verifies that amounts render with a comma by default
// (German EUR) and with a dot when decimalSeparator="dot", while the EPC QR
// payload always keeps the dot form (checked elsewhere). Skipped when the
// LaTeX/poppler toolchain is unavailable.
func TestDecimalSeparator(t *testing.T) {
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
	data, err := os.ReadFile(filepath.Join("invoices", "2026-001.json"))
	if err != nil {
		t.Fatalf("read invoice: %v", err)
	}

	render := func(sep string) string {
		var req InvoiceRequest
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		req.DecimalSeparator = sep
		pdf, cleanup, err := buildDocument(req, profiles[req.ProfileKey], docConfigs["invoice"], req.Design, "invoice")
		if err != nil {
			t.Fatalf("buildDocument (%q): %v", sep, err)
		}
		defer cleanup()
		out, err := exec.Command("pdftotext", pdf, "-").Output()
		if err != nil {
			t.Fatalf("pdftotext (%q): %v", sep, err)
		}
		return string(out)
	}

	// Default (empty) => comma.
	if s := render(""); !strings.Contains(s, "518,88") || strings.Contains(s, "518.88") {
		t.Errorf("default should render comma 518,88, not dot")
	}
	// Explicit dot.
	if s := render("dot"); !strings.Contains(s, "518.88") || strings.Contains(s, "518,88") {
		t.Errorf("dot mode should render 518.88 with dot")
	}
}
