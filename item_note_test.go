package main

import "testing"

// The optional per-item note must land inside the description cell as
// \FeeNote{…}, after the VAT label, escaped, and must vanish entirely when
// empty (so existing invoices render byte-identically).
func TestItemDescriptionNote(t *testing.T) {
	cases := []struct {
		name        string
		item        LineItem
		showVatLabl bool
		want        string
	}{
		{"no note", LineItem{Description: "Kaffee"}, false, `Kaffee`},
		{"blank note", LineItem{Description: "Kaffee", Note: "  \n "}, false, `Kaffee`},
		{"note", LineItem{Description: "Kaffee", Note: "Arabica, 100 % Bio"}, false,
			`Kaffee\FeeNote{Arabica, 100 \% Bio}`},
		{"note after vat label", LineItem{Description: "Kaffee", VatRate: "7", Note: "Bio"}, true,
			`Kaffee (7\% MwSt.)\FeeNote{Bio}`},
		{"newlines collapse", LineItem{Description: "Kaffee", Note: "Zeile 1\nZeile 2"}, false,
			`Kaffee\FeeNote{Zeile 1 Zeile 2}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := itemDescription(c.item, c.showVatLabl); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
