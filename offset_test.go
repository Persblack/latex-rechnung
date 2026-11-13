package main

import (
	"strings"
	"testing"
)

// Offset is an Aufrechnung after the total: the QR amount drops, the VAT does not.
func TestOffsetLowersPayableNotVat(t *testing.T) {
	req := InvoiceRequest{UseVat: true, PaymentMode: "transfer", OffsetAmount: "95.00",
		Items: []LineItem{{UnitPrice: "696.64", Quantity: "1", VatRate: "7"}}}
	b := computeVatBreakdown(req.Items, true)
	if b[0].VatCents != 4876 {
		t.Fatalf("vat = %d, want 4876", b[0].VatCents)
	}
	gross := b[0].NetCents + b[0].VatCents
	payable := gross - parseCents(req.OffsetAmount)
	if formatCents(payable) != "650.40" {
		t.Fatalf("payable = %s, want 650.40", formatCents(payable))
	}
	if !strings.Contains(buildEPCPayload("A", "DE50500105175450379340", "INGDDEFFXXX", req, formatCents(payable)), "EUR650.40") {
		t.Fatal("EPC payload must carry the payable amount")
	}
}
