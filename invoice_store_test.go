package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// setupInvoiceStore points the store at a temp dir and clears the map, so the
// test never touches the real invoices/ directory.
func setupInvoiceStore(t *testing.T) {
	t.Helper()
	invoiceDir = t.TempDir()
	invoicesMu.Lock()
	invoices = map[string]*InvoiceRequest{}
	invoicesMu.Unlock()
}

func postInvoice(req InvoiceRequest) *httptest.ResponseRecorder {
	body, _ := json.Marshal(req)
	r := httptest.NewRequest(http.MethodPost, "/api/invoices", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handleSaveInvoice(w, r)
	return w
}

func TestInvoiceStoreCreateConflictUpdate(t *testing.T) {
	setupInvoiceStore(t)

	base := InvoiceRequest{
		ProfileKey:       "rico-solo",
		Design:           "modern",
		InvoiceReference: "2026-001",
		CustomerCompany:  "Remexian Pharma GmbH",
		Items:            []LineItem{{Description: "Hosting", UnitPrice: "180.00", Quantity: "1"}},
	}

	// First create succeeds.
	if w := postInvoice(base); w.Code != http.StatusOK {
		t.Fatalf("create: want 200, got %d (%s)", w.Code, w.Body.String())
	}

	// Second create with the same number is rejected (the "Fehler werfen" rule).
	if w := postInvoice(base); w.Code != http.StatusConflict {
		t.Fatalf("duplicate create: want 409, got %d (%s)", w.Code, w.Body.String())
	}

	// PUT updates the existing record.
	updated := base
	updated.Items = []LineItem{{Description: "Hosting", UnitPrice: "200.00", Quantity: "1"}}
	body, _ := json.Marshal(updated)
	r := httptest.NewRequest(http.MethodPut, "/api/invoices/2026-001", bytes.NewReader(body))
	r.SetPathValue("key", "2026-001")
	w := httptest.NewRecorder()
	handleUpdateInvoice(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d (%s)", w.Code, w.Body.String())
	}

	// GET returns the updated price.
	r = httptest.NewRequest(http.MethodGet, "/api/invoices/2026-001", nil)
	r.SetPathValue("key", "2026-001")
	w = httptest.NewRecorder()
	handleInvoice(w, r)
	var got InvoiceRequest
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("get decode: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].UnitPrice != "200.00" {
		t.Fatalf("update not persisted: %+v", got.Items)
	}
}

func TestInvoiceStoreRejectsEmptyReference(t *testing.T) {
	setupInvoiceStore(t)
	if w := postInvoice(InvoiceRequest{ProfileKey: "rico-solo"}); w.Code != http.StatusBadRequest {
		t.Fatalf("empty reference: want 400, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestInvoiceUpdateRejectsKeyMismatch(t *testing.T) {
	setupInvoiceStore(t)
	body, _ := json.Marshal(InvoiceRequest{InvoiceReference: "2026-999"})
	r := httptest.NewRequest(http.MethodPut, "/api/invoices/2026-001", bytes.NewReader(body))
	r.SetPathValue("key", "2026-001")
	w := httptest.NewRecorder()
	handleUpdateInvoice(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("key mismatch: want 400, got %d (%s)", w.Code, w.Body.String())
	}
}
