package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVatRoundsDownPerRate pins the rounding rule: VAT is computed once per
// rate on the net sum and rounded DOWN (never shows more than owed, § 14c UStG).
func TestVatRoundsDownPerRate(t *testing.T) {
	items := []LineItem{
		{UnitPrice: "10.05", Quantity: "1", VatRate: "19"},
		{UnitPrice: "10.05", Quantity: "1", VatRate: "19"},
		{UnitPrice: "10.52", Quantity: "1", VatRate: "7"},
	}
	b := computeVatBreakdown(items, true)
	if len(b) != 2 {
		t.Fatalf("want 2 rates, got %+v", b)
	}
	// 7 %: 1052 × 7 % = 73,64 Cent → 73.
	if b[0].Rate != 7 || b[0].VatCents != 73 {
		t.Errorf("7%%: want 73 cents, got %+v", b[0])
	}
	// 19 %: 2010 × 19 % = 381,9 Cent → 381 (per-line floor would give 380,
	// kaufmännisch 382).
	if b[1].Rate != 19 || b[1].VatCents != 381 {
		t.Errorf("19%%: want 381 cents, got %+v", b[1])
	}
}

// TestClassicUsesGoVat guards the seam: classic's single-rate total block must
// print the Go-computed VAT, not recompute it in LaTeX.
func TestClassicUsesGoVat(t *testing.T) {
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
	req.Design = "classic"
	req.Items = []LineItem{
		{Description: "A", UnitPrice: "10.05", Quantity: "1", VatRate: "19"},
		{Description: "B", UnitPrice: "10.05", Quantity: "1", VatRate: "19"},
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
	for _, want := range []string{"20,10", "3,81", "23,91"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in classic invoice text", want)
		}
	}
}
