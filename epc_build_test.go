package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBuildInvoiceWithEPCQR exercises the full build path (EPC QR render +
// pdflatex) for the modern design and asserts the QR image lands in the build
// dir. Skipped automatically when pdflatex is not on PATH.
func TestBuildInvoiceWithEPCQR(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	if err := loadProfiles("profiles"); err != nil {
		t.Fatalf("loadProfiles: %v", err)
	}
	p := profiles["rico-solo"]
	if p == nil {
		t.Fatal("profile rico-solo not loaded")
	}

	req := InvoiceRequest{
		ProfileKey:       "rico-solo",
		Design:           "modern",
		InvoiceDate:      "2026-06-03",
		InvoiceReference: "2026-014",
		CustomerCompany:  "Beispiel GmbH",
		CustomerName:     "Max Muster",
		CustomerStreet:   "Teststrasse 1",
		CustomerZIP:      "12345",
		CustomerCity:     "Berlin",
		PaymentMode:      "transfer",
		Items:            []LineItem{{Description: "Beratung", UnitPrice: "350.00", Quantity: "1"}},
	}

	pdfPath, cleanup, err := buildDocument(req, p, docConfigs["invoice"], "modern", "invoice")
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(filepath.Dir(pdfPath), "epc-qr.png")); err != nil {
		t.Fatalf("expected epc-qr.png in build dir: %v", err)
	}
}

// TestBuildInvoiceCashNoQR verifies that cash payment produces no QR file, so
// the design falls back to its placeholder.
func TestBuildInvoiceCashNoQR(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	if err := loadProfiles("profiles"); err != nil {
		t.Fatalf("loadProfiles: %v", err)
	}
	p := profiles["rico-solo"]

	req := InvoiceRequest{
		ProfileKey:       "rico-solo",
		InvoiceReference: "2026-015",
		CustomerName:     "Max Muster",
		CustomerStreet:   "Teststrasse 1",
		CustomerZIP:      "12345",
		CustomerCity:     "Berlin",
		PaymentMode:      "cash_due",
		Items:            []LineItem{{Description: "Beratung", UnitPrice: "350.00", Quantity: "1"}},
	}

	pdfPath, cleanup, err := buildDocument(req, p, docConfigs["invoice"], "modern", "invoice")
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	defer cleanup()

	if _, err := os.Stat(filepath.Join(filepath.Dir(pdfPath), "epc-qr.png")); err == nil {
		t.Fatal("expected no epc-qr.png for cash payment, but file exists")
	}
}
