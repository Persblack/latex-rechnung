package main

import (
	"strings"
	"testing"
)

func TestBuildEPCPayloadRicoSolo(t *testing.T) {
	req := InvoiceRequest{InvoiceReference: "2026-014"}

	got := buildEPCPayload("Rico Klatte", "DE50 5001 0517 5450 3793 40", "INGDDEFFXXX", req, "398.50")
	want := strings.Join([]string{
		"BCD",
		"002",
		"1",
		"SCT",
		"INGDDEFFXXX",
		"Rico Klatte",
		"DE50500105175450379340",
		"EUR398.50",
		"",
		"",
		"Rechnung 2026-014 Rico Klatte",
	}, "\n")
	if got != want {
		t.Fatalf("payload mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestBuildEPCPayloadSkipsWithoutIBANorAmount(t *testing.T) {
	if got := buildEPCPayload("X", "", "", InvoiceRequest{}, "398.50"); got != "" {
		t.Fatalf("expected empty payload without IBAN, got %q", got)
	}
	if got := buildEPCPayload("X", "DE50500105175450379340", "", InvoiceRequest{}, "0.00"); got != "" {
		t.Fatalf("expected empty payload for zero amount, got %q", got)
	}
}
