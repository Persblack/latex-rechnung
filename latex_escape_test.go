package main

import (
	"os/exec"
	"strings"
	"testing"
)

// babel-german makes `"` an active character (the `"a` → ä shorthand family).
// An unescaped `"` in user text therefore does not reach the typesetter as a
// character at all: it starts a shorthand that swallows the closing brace of
// the surrounding macro argument, and pdflatex dies with
// `Argument of \language@active@arg" has an extra }`. Every design loads
// babel ngerman, so this has to be handled where all other specials are —
// in latexEscape.
func TestLatexEscapeQuotes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"straight quotes", `Wort-Bild Marke "Kleinstadt Roastery"`,
			`Wort-Bild Marke \textquotedbl{}Kleinstadt Roastery\textquotedbl{}`},
		{"inch mark", `Display 27"`, `Display 27\textquotedbl{}`},
		{"german quotes pass through", `Marke „Kleinstadt Roastery“`, `Marke „Kleinstadt Roastery“`},
		{"other specials still escaped", `100 % & R_D #1 $5`, `100 \% \& R\_D \#1 \$5`},
		{"umlauts untouched", `Pfälzer Grüße`, `Pfälzer Grüße`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := latexEscape(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The escape has to survive the whole build, in every design: a description
// and a note containing `"` must produce a PDF rather than a pdflatex abort.
func TestBuildInvoiceWithQuotesInText(t *testing.T) {
	if _, err := exec.LookPath("pdflatex"); err != nil {
		t.Skip("pdflatex not installed")
	}
	if err := loadProfiles("profiles"); err != nil {
		t.Fatalf("loadProfiles: %v", err)
	}
	if err := loadBankAccounts("bankaccounts.json"); err != nil {
		t.Fatalf("loadBankAccounts: %v", err)
	}
	if err := loadDesigns("designs"); err != nil {
		t.Fatalf("loadDesigns: %v", err)
	}

	for _, design := range []string{"classic", "modern", "nonstop"} {
		t.Run(design, func(t *testing.T) {
			req := InvoiceRequest{
				ProfileKey:       "nonstop-performance",
				Design:           design,
				InvoiceDate:      "10.06.2025",
				InvoiceReference: "QUOTE-1",
				InvoiceText:      `Betrifft die Marke "Kleinstadt Roastery".`,
				CustomerName:     "Max Muster",
				CustomerStreet:   "Teststrasse 1",
				CustomerZIP:      "12345",
				CustomerCity:     "Berlin",
				PaymentMode:      "transfer",
				UseVat:           true,
				Items: []LineItem{{
					Description: `Wort-Bild Marke "Kleinstadt Roastery"`,
					Note:        `inklusive Übertragung der Rechte an "Logo" und "Schriftzug"`,
					UnitPrice:   "1000.00",
					Quantity:    "1",
					VatRate:     "19",
				}},
			}

			p := profiles[req.ProfileKey]
			if p == nil {
				t.Fatalf("profile %s not loaded", req.ProfileKey)
			}
			_, cleanup, err := buildDocument(req, p, docConfigs["invoice"], design, "invoice")
			if cleanup != nil {
				defer cleanup()
			}
			if err != nil {
				line := err.Error()
				if i := strings.Index(line, "! "); i >= 0 {
					line = line[i:]
					if j := strings.Index(line, "\n"); j >= 0 {
						line = line[:j]
					}
				}
				t.Fatalf("buildDocument(%s) failed: %s", design, line)
			}
		})
	}
}
