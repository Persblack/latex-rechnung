package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBuildStoredRemexianInvoices loads the two saved Remexian invoices and
// builds each into a PDF via the real build path, copying the results to /tmp
// for visual inspection. Skipped when pdflatex is unavailable.
func TestBuildStoredRemexianInvoices(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	if err := loadProfiles("profiles"); err != nil {
		t.Fatalf("loadProfiles: %v", err)
	}

	for _, ref := range []string{"2026-001", "2026-002"} {
		data, err := os.ReadFile(filepath.Join("invoices", ref+".json"))
		if err != nil {
			t.Fatalf("read invoice %s: %v", ref, err)
		}
		var req InvoiceRequest
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("unmarshal invoice %s: %v", ref, err)
		}
		p := profiles[req.ProfileKey]
		if p == nil {
			t.Fatalf("profile %q for invoice %s not loaded", req.ProfileKey, ref)
		}

		pdfPath, cleanup, err := buildDocument(req, p, docConfigs["invoice"], req.Design, "invoice")
		if err != nil {
			t.Fatalf("buildDocument %s: %v", ref, err)
		}
		// QR must be present (transfer payment + IBAN) and the PDF non-trivial.
		_, qrErr := os.Stat(filepath.Join(filepath.Dir(pdfPath), "epc-qr.png"))
		info, pdfErr := os.Stat(pdfPath)
		cleanup()
		if qrErr != nil {
			t.Fatalf("invoice %s: expected epc-qr.png: %v", ref, qrErr)
		}
		if pdfErr != nil || info.Size() < 1000 {
			t.Fatalf("invoice %s: pdf missing or too small: %v", ref, pdfErr)
		}
	}
}
