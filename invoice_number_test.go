package main

import (
	"testing"
	"time"
)

func TestNextInvoiceReference(t *testing.T) {
	saved := invoices
	defer func() { invoices = saved }()
	invoices = map[string]*InvoiceRequest{
		"202608-401":  {ProfileKey: "milky", InvoiceReference: "202608-401"},
		"202609-1":    {ProfileKey: "milky", InvoiceReference: "202609-1"},
		"so-2026-001": {ProfileKey: "selim", InvoiceReference: "SO-2026-001"},
		"202609-2":    {ProfileKey: "other", InvoiceReference: "202609-2"},
	}
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.Local)
	cases := map[string]string{
		"milky": "202609-402", // continues its highest number, not the month's
		"selim": "202609-3",   // 2 is held by another profile → skipped
		"fresh": "202609-3",   // first invoice; 1 and 2 already taken
	}
	for profile, want := range cases {
		if got := nextInvoiceReference(profile, now); got != want {
			t.Errorf("%s: want %s, got %s", profile, want, got)
		}
	}
}
