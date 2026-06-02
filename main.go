package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"

	qrcode "github.com/skip2/go-qrcode"
)

//go:embed static all:templates all:designs
var embedded embed.FS

// defaultDesign is used when an InvoiceRequest omits the design field.
// Keeps existing /generate and /lieferschein clients (and the embedded
// dashboard prior to upgrade) working unchanged.
const defaultDesign = "classic"

// Profile holds sender identity data loaded from a profiles/*.json file.
type Profile struct {
	Name               string   `json:"name"`
	TaxID              string   `json:"taxID"`
	SenderName         string   `json:"senderName"`
	SenderCompanyLines []string `json:"senderCompanyLines"`
	SenderStreet       string   `json:"senderStreet"`
	SenderZIP          string   `json:"senderZIP"`
	SenderCity         string   `json:"senderCity"`
	SenderTelephone    string   `json:"senderTelephone"`
	SenderMobilephone  string   `json:"senderMobilephone"`
	SenderEmail        string   `json:"senderEmail"`
	SenderWeb          string   `json:"senderWeb"`
	AccountIBAN        string   `json:"accountIBAN"`
	AccountBIC         string   `json:"accountBIC"`
	AccountBankName    string   `json:"accountBankName"`
	Logo               string   `json:"logo"`
	ShortName          string   `json:"shortName"` // fallback wordmark when logo is missing (e.g. "rico", "frameway")
	VatID              string   `json:"vatID"`
	VatRate            int      `json:"vatRate"` // optional; defaults to 19 when VatID is set
}

// Recipient holds saved customer (Empfänger) Stammdaten loaded from a
// recipients/*.json file. The first five fields map 1:1 to the customer*
// fields of an InvoiceRequest; the rest are stored-only metadata that is
// kept for reference and NOT printed on the invoice.
type Recipient struct {
	Company   string `json:"company"`
	Name      string `json:"name"`
	Street    string `json:"street"`
	ZIP       string `json:"zip"`
	City      string `json:"city"`
	VatID     string `json:"vatID"`
	TaxNo     string `json:"taxNo"`
	Register  string `json:"register"`
	Directors string `json:"directors"`
	Email     string `json:"email"`
	Telephone string `json:"telephone"`
	Fax       string `json:"fax"`
}

type LineItem struct {
	Description string `json:"description"`
	UnitPrice   string `json:"unitPrice"`
	Quantity    string `json:"quantity"`
	VatRate     string `json:"vatRate"` // "", "7", or "19"

	// Optional per-line discount (Nachlass). DiscountKind selects how
	// DiscountValue is interpreted; empty kind/value means no discount.
	DiscountValue string `json:"discountValue"` // "10" (percent) or "9.00" (amount)
	DiscountKind  string `json:"discountKind"`  // "percent" | "amount" | ""

	// Computed for template rendering only (never read from the request).
	HasDiscount       bool   `json:"-"`
	DiscountLabel     string `json:"-"` // e.g. "10\,\%" for percent; empty for fixed amount
	DiscountAmountStr string `json:"-"` // positive discount amount in €, e.g. "9.00"
}

// VatBreakdown holds the aggregated net/vat/gross amounts for one VAT rate.
type VatBreakdown struct {
	Rate       int    // e.g. 0, 7, 19
	NetCents   int64  // net sum for this rate in cents
	VatCents   int64  // VAT amount for this rate in cents
	GrossCents int64  // gross = net + vat in cents
	NetStr     string // "1234.56"
	VatStr     string // "234.56"
	GrossStr   string // "1469.12"
}

type InvoiceRequest struct {
	ProfileKey        string     `json:"profileKey"`
	Design            string     `json:"design"`
	InvoiceDate       string     `json:"invoiceDate"`
	PayDate           string     `json:"payDate"`
	InvoiceReference  string     `json:"invoiceReference"`
	InvoiceSalutation string     `json:"invoiceSalutation"`
	InvoiceText       string     `json:"invoiceText"`
	InvoiceEnclosures string     `json:"invoiceEnclosures"`
	InvoiceClosing    string     `json:"invoiceClosing"`
	CustomerCompany   string     `json:"customerCompany"`
	CustomerName      string     `json:"customerName"`
	CustomerStreet    string     `json:"customerStreet"`
	CustomerZIP       string     `json:"customerZIP"`
	CustomerCity      string     `json:"customerCity"`
	ProjectTitle      string     `json:"projectTitle"`
	UseVat            bool       `json:"useVat"`
	HideQR            bool       `json:"hideQR"`        // when true, designs suppress the QR-code block. Default false = QR rendered.
	Language          string     `json:"language"`      // "de" (default) or "en". Empty => "de".
	PaymentMode       string     `json:"paymentMode"`   // "transfer" (default) | "cash_due" | "cash_paid"
	CashPaidDate      string     `json:"cashPaidDate"`  // ISO or DE date string, only used when paymentMode=cash_paid
	Clauses           []string   `json:"clauses"`       // any of: "warranty_excluded", "retention_of_title", "late_fee_warning"
	Items             []LineItem `json:"items"`
}

// TemplateData is passed to the LaTeX templates. All string fields are
// pre-escaped for LaTeX, except SenderEmail and SenderWeb which are
// used inside \href / \url and handled by the hyperref/url packages.
type TemplateData struct {
	TaxID             string
	SenderName        string
	SenderCompany     string
	SenderStreet      string
	SenderZIP         string
	SenderCity        string
	SenderTelephone   string
	SenderMobilephone string
	SenderEmail       string
	SenderWeb         string
	AccountRCPT       string
	AccountIBAN       string
	AccountBIC        string
	AccountBankName   string
	VatID             string
	VatRate           int
	InvoiceDate       string
	PayDate           string
	InvoiceReference  string
	InvoiceSalutation string
	InvoiceText       string
	InvoiceEnclosures string
	InvoiceClosing    string
	CustomerCompany   string
	CustomerName      string
	CustomerStreet    string
	CustomerZIP       string
	CustomerCity      string
	ProjectTitle      string
	Items             []LineItem

	// VAT breakdown — computed per request, see computeVatBreakdown.
	VatBreakdown        []VatBreakdown
	NetTotalStr         string // sum of all net amounts  e.g. "350.00"
	VatTotalStr         string // sum of all VAT amounts  e.g.  "48.50"
	GrossTotalStr       string // gross total             e.g. "398.50"
	HasMultipleVatRates bool   // true when >1 distinct non-zero rate appears
	HasAnyVat           bool   // true when useVat=true and at least one item has vatRate>0

	// Branding — designs use these to render a logo image or a wordmark fallback.
	ShortName string // profile.shortName, escaped for LaTeX; intended as wordmark when HasLogo is false
	HasLogo   bool   // true when logo file was found and copied to tmpDir as logo.png

	// QR-Code visibility (per-request UI toggle).
	ShowQR bool // true when the design should render the QR-code block. Mirrors !req.HideQR.

	// HasQRFile is true when a real EPC/Girocode QR image (epc-qr.png) was
	// rendered into the build dir. Designs branch on \ifHasQRFile to
	// \includegraphics it; when false they fall back to a placeholder.
	HasQRFile bool

	// Output language. Default: German. English when req.Language == "en".
	IsEnglish bool

	// Project title visibility.
	HasProjectTitle bool // true when req.ProjectTitle != ""

	// Payment mode flags — exactly one is true per request.
	PaymentMode       string // "transfer" | "cash_due" | "cash_paid"
	IsPaymentTransfer bool
	IsPaymentCashDue  bool
	IsPaymentCashPaid bool
	CashPaidDate      string // latex-escaped date shown in cash_paid receipts

	// Optional legal clause flags.
	ShowWarrantyExcluded bool
	ShowRetentionOfTitle bool
	ShowLateFeeWarning   bool
}

// Design describes one visual layout variant. The actual .tex/.sty/.def
// files live under designs/<Key>/ inside the embedded FS; design.json
// supplies the human-readable metadata loaded by loadDesigns.
type Design struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Supports    []string `json:"supports"`
}

var profiles = map[string]*Profile{}
var designs = map[string]*Design{}

// recipients is mutated at runtime via the save/delete endpoints, so unlike
// profiles/designs it must be guarded. recipientDir is where files are read
// from and written to.
const recipientDir = "recipients"

var (
	recipientsMu sync.RWMutex
	recipients   = map[string]*Recipient{}
)

func main() {
	if err := loadProfiles("profiles"); err != nil {
		log.Printf("warning: could not load profiles: %v", err)
	}
	if err := loadDesigns("designs"); err != nil {
		log.Printf("warning: could not load designs: %v", err)
	}
	if err := loadRecipients(recipientDir); err != nil {
		log.Printf("warning: could not load recipients: %v", err)
	}

	mux := http.NewServeMux()

	staticFS, _ := fs.Sub(embedded, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /api/profiles", handleProfiles)
	mux.HandleFunc("GET /api/profiles/{key}", handleProfile)
	mux.HandleFunc("GET /api/recipients", handleRecipients)
	mux.HandleFunc("GET /api/recipients/{key}", handleRecipient)
	mux.HandleFunc("POST /api/recipients", handleSaveRecipient)
	mux.HandleFunc("DELETE /api/recipients/{key}", handleDeleteRecipient)
	mux.HandleFunc("GET /api/designs", handleDesigns)
	mux.HandleFunc("GET /api/designs/{key}", handleDesign)
	mux.HandleFunc("POST /generate", handleGenerate)
	mux.HandleFunc("POST /lieferschein", handleLieferschein)

	log.Println("Listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func loadProfiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			log.Printf("skipping %s: %v", e.Name(), err)
			continue
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			log.Printf("skipping %s (invalid JSON): %v", e.Name(), err)
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		profiles[key] = &p
		log.Printf("loaded profile %q (%s)", key, p.Name)
	}
	return nil
}

func handleProfiles(w http.ResponseWriter, r *http.Request) {
	type summary struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	}
	list := make([]summary, 0, len(profiles))
	for k, p := range profiles {
		list = append(list, summary{Key: k, Name: p.Name})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	p, ok := profiles[key]
	if !ok {
		http.Error(w, "profile not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func loadRecipients(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no recipients saved yet — not an error
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			log.Printf("skipping %s: %v", e.Name(), err)
			continue
		}
		var rcpt Recipient
		if err := json.Unmarshal(data, &rcpt); err != nil {
			log.Printf("skipping %s (invalid JSON): %v", e.Name(), err)
			continue
		}
		key := strings.TrimSuffix(e.Name(), ".json")
		recipients[key] = &rcpt
		log.Printf("loaded recipient %q (%s)", key, recipientLabel(&rcpt))
	}
	return nil
}

// recipientLabel returns the company name, falling back to the contact name.
func recipientLabel(r *Recipient) string {
	if strings.TrimSpace(r.Company) != "" {
		return r.Company
	}
	return r.Name
}

// slugify turns an arbitrary label into a filesystem- and URL-safe key
// containing only [a-z0-9-]. This is the sole defense against path traversal
// for the recipient files, so it must never emit "/", "." or "..".
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == 'ä':
			b.WriteString("ae")
			prevDash = false
		case r == 'ö':
			b.WriteString("oe")
			prevDash = false
		case r == 'ü':
			b.WriteString("ue")
			prevDash = false
		case r == 'ß':
			b.WriteString("ss")
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func handleRecipients(w http.ResponseWriter, r *http.Request) {
	type summary struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	recipientsMu.RLock()
	list := make([]summary, 0, len(recipients))
	for k, rcpt := range recipients {
		list = append(list, summary{Key: k, Label: recipientLabel(rcpt)})
	}
	recipientsMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleRecipient(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	recipientsMu.RLock()
	rcpt, ok := recipients[key]
	recipientsMu.RUnlock()
	if !ok {
		http.Error(w, "recipient not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rcpt)
}

func handleSaveRecipient(w http.ResponseWriter, r *http.Request) {
	var rcpt Recipient
	if err := json.NewDecoder(r.Body).Decode(&rcpt); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(rcpt.Name) == "" && strings.TrimSpace(rcpt.Company) == "" {
		http.Error(w, "Firma oder Name erforderlich", http.StatusBadRequest)
		return
	}
	key := slugify(recipientLabel(&rcpt))
	if key == "" {
		http.Error(w, "konnte keinen gültigen Schlüssel ableiten", http.StatusBadRequest)
		return
	}

	data, err := json.MarshalIndent(&rcpt, "", "  ")
	if err != nil {
		http.Error(w, "encode error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(recipientDir, 0o755); err != nil {
		http.Error(w, "konnte Verzeichnis nicht anlegen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(recipientDir, key+".json"), append(data, '\n'), 0o644); err != nil {
		http.Error(w, "konnte nicht speichern: "+err.Error(), http.StatusInternalServerError)
		return
	}

	recipientsMu.Lock()
	recipients[key] = &rcpt
	recipientsMu.Unlock()
	log.Printf("saved recipient %q (%s)", key, recipientLabel(&rcpt))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}{Key: key, Label: recipientLabel(&rcpt)})
}

func handleDeleteRecipient(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	// Re-slugify defensively: the path key must round-trip to itself, else
	// it never matched a real file and could be a traversal attempt.
	if key == "" || key != slugify(key) {
		http.Error(w, "ungültiger Schlüssel", http.StatusBadRequest)
		return
	}
	recipientsMu.Lock()
	_, ok := recipients[key]
	if ok {
		delete(recipients, key)
	}
	recipientsMu.Unlock()
	if !ok {
		http.Error(w, "recipient not found", http.StatusNotFound)
		return
	}
	if err := os.Remove(filepath.Join(recipientDir, key+".json")); err != nil && !os.IsNotExist(err) {
		http.Error(w, "konnte nicht löschen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("deleted recipient %q", key)
	w.WriteHeader(http.StatusNoContent)
}

func loadDesigns(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifest := filepath.Join(dir, e.Name(), "design.json")
		data, err := os.ReadFile(manifest)
		if err != nil {
			log.Printf("skipping design %q: no design.json (%v)", e.Name(), err)
			continue
		}
		var d Design
		if err := json.Unmarshal(data, &d); err != nil {
			log.Printf("skipping design %q (invalid JSON): %v", e.Name(), err)
			continue
		}
		d.Key = e.Name()
		designs[d.Key] = &d
		log.Printf("loaded design %q (%s)", d.Key, d.Name)
	}
	return nil
}

func handleDesigns(w http.ResponseWriter, r *http.Request) {
	list := make([]Design, 0, len(designs))
	for _, d := range designs {
		list = append(list, *d)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleDesign(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	d, ok := designs[key]
	if !ok {
		http.Error(w, "design not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// designSupports reports whether design d covers the given docType.
// An empty supports slice is treated as "supports all" for backwards-compat
// with hand-authored design.json files that omit the field.
func designSupports(d *Design, docType string) bool {
	if len(d.Supports) == 0 {
		return true
	}
	for _, s := range d.Supports {
		if s == docType {
			return true
		}
	}
	return false
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	handleDoc(w, r, "invoice")
}

func handleLieferschein(w http.ResponseWriter, r *http.Request) {
	handleDoc(w, r, "lieferschein")
}

// docConfig is the per-docType build recipe. The build copies the mainTex
// plus every other file in designs/<designKey>/ except design.json and
// other docTypes' mainTex files, so each design ships only what it needs.
type docConfig struct {
	mainTexName string // file name only, e.g. "_main.tex"
	itemsTmpl   string // shared template path inside embedded FS, e.g. "templates/_invoice.tex.tmpl"
	itemsOut    string // rendered snippet file name written into the build dir
	outputPDF   string // pdflatex output name (derived from \jobname)
	filePrefix  string // HTTP download filename prefix
}

var docConfigs = map[string]docConfig{
	"invoice": {
		mainTexName: "_main.tex",
		itemsTmpl:   "templates/_invoice.tex.tmpl",
		itemsOut:    "_invoice.tex",
		outputPDF:   "_main.pdf",
		filePrefix:  "rechnung",
	},
	"lieferschein": {
		mainTexName: "_lieferschein_main.tex",
		itemsTmpl:   "templates/_lieferschein_items.tex.tmpl",
		itemsOut:    "_lieferschein_items.tex",
		outputPDF:   "_lieferschein_main.pdf",
		filePrefix:  "lieferschein",
	},
}

// otherDocMainTeXNames returns the mainTexName of every docType *other*
// than the one currently being built, so the build dir for an invoice
// doesn't get polluted with the lieferschein's _lieferschein_main.tex.
func otherDocMainTeXNames(currentDocType string) map[string]bool {
	excluded := map[string]bool{}
	for k, c := range docConfigs {
		if k != currentDocType {
			excluded[c.mainTexName] = true
		}
	}
	return excluded
}

func handleDoc(w http.ResponseWriter, r *http.Request, docType string) {
	var req InvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	p, ok := profiles[req.ProfileKey]
	if !ok {
		http.Error(w, "unknown profile: "+req.ProfileKey, http.StatusBadRequest)
		return
	}

	// Resolve design key with backwards-compat fallback. An empty field
	// keeps the pre-refactor request shape valid.
	designKey := req.Design
	if designKey == "" {
		designKey = defaultDesign
	}
	d, ok := designs[designKey]
	if !ok {
		http.Error(w, "unknown design: "+designKey, http.StatusBadRequest)
		return
	}
	if !designSupports(d, docType) {
		http.Error(w, fmt.Sprintf("design %q does not support %s", designKey, docType), http.StatusBadRequest)
		return
	}

	cfg := docConfigs[docType]
	pdfPath, cleanup, err := buildDocument(req, p, cfg, designKey, docType)
	defer cleanup()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	f, err := os.Open(pdfPath)
	if err != nil {
		http.Error(w, "could not read PDF: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	filename := fmt.Sprintf("%s-%s.pdf", cfg.filePrefix, req.InvoiceReference)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	io.Copy(w, f)
}

// parseCents converts a decimal string amount to integer cents.
// "150.00" → 15000, "8" → 800, "0.5" → 50.
// On parse failure it returns 0 and logs a warning.
func parseCents(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.SplitN(s, ".", 2)
	intPart, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		log.Printf("parseCents: cannot parse integer part of %q: %v", s, err)
		return 0
	}
	cents := intPart * 100
	if len(parts) == 2 {
		frac := parts[1]
		// Normalise to exactly 2 decimal digits.
		switch len(frac) {
		case 0:
			// nothing
		case 1:
			frac += "0"
		default:
			frac = frac[:2]
		}
		fracVal, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			log.Printf("parseCents: cannot parse fractional part of %q: %v", s, err)
			return cents
		}
		if intPart < 0 {
			cents -= fracVal
		} else {
			cents += fracVal
		}
	}
	return cents
}

// formatCents converts integer cents back to a decimal string with 2 places.
// 15000 → "150.00", 234 → "2.34".
func formatCents(c int64) string {
	neg := c < 0
	if neg {
		c = -c
	}
	s := fmt.Sprintf("%d.%02d", c/100, c%100)
	if neg {
		return "-" + s
	}
	return s
}

// roundHalfUp rounds x to the nearest integer, with ties going away from zero
// (kaufmännische Rundung / standard German invoice rounding).
func roundHalfUp(x float64) int64 {
	return int64(math.Floor(x + 0.5))
}

// lineDiscountCents returns the discount amount (in € cents) for a single line,
// given that line's net before discount. A percentage discount is rounded
// kaufmännisch; a fixed amount is capped at the line net so a line can never go
// negative. Returns 0 when no discount is set.
func lineDiscountCents(item LineItem, lineNetCents int64) int64 {
	v := strings.TrimSpace(item.DiscountValue)
	if v == "" {
		return 0
	}
	switch item.DiscountKind {
	case "percent":
		p, err := strconv.ParseFloat(v, 64)
		if err != nil || p <= 0 {
			return 0
		}
		if p > 100 {
			p = 100
		}
		return roundHalfUp(float64(lineNetCents) * p / 100.0)
	case "amount":
		c := parseCents(v)
		if c < 0 {
			c = 0
		}
		if c > lineNetCents {
			c = lineNetCents
		}
		return c
	}
	return 0
}

// computeVatBreakdown aggregates per-item net/vat/gross amounts grouped by VAT
// rate and returns them sorted by rate ascending.
func computeVatBreakdown(items []LineItem, useVat bool) []VatBreakdown {
	type accumulator struct {
		netCents int64
		vatCents int64
	}
	rateMap := map[int]*accumulator{}

	for _, item := range items {
		unitCents := parseCents(item.UnitPrice)
		qtyCents := parseCents(item.Quantity)
		// Fixed-point multiply: unitCents (€ cents) × qtyCents (hundredths of a
		// unit) ÷ 100 = line net in € cents.
		lineNetCents := unitCents * qtyCents / 100

		// Apply the per-line discount BEFORE VAT: the discounted net is the
		// taxable base (Entgelt, §10 UStG), so VAT must be computed on it.
		lineNetCents -= lineDiscountCents(item, lineNetCents)

		rate := 0
		if useVat && item.VatRate != "" {
			r, err := strconv.Atoi(strings.TrimSpace(item.VatRate))
			if err != nil {
				log.Printf("computeVatBreakdown: invalid vatRate %q: %v", item.VatRate, err)
			} else {
				rate = r
			}
		}

		// Kaufmännisch gerundete MwSt pro Zeile.
		lineVatCents := roundHalfUp(float64(lineNetCents) * float64(rate) / 100.0)

		if _, ok := rateMap[rate]; !ok {
			rateMap[rate] = &accumulator{}
		}
		rateMap[rate].netCents += lineNetCents
		rateMap[rate].vatCents += lineVatCents
	}

	// Collect and sort by rate ascending.
	rates := make([]int, 0, len(rateMap))
	for r := range rateMap {
		rates = append(rates, r)
	}
	sort.Ints(rates)

	breakdown := make([]VatBreakdown, 0, len(rates))
	for _, r := range rates {
		acc := rateMap[r]
		gross := acc.netCents + acc.vatCents
		breakdown = append(breakdown, VatBreakdown{
			Rate:       r,
			NetCents:   acc.netCents,
			VatCents:   acc.vatCents,
			GrossCents: gross,
			NetStr:     formatCents(acc.netCents),
			VatStr:     formatCents(acc.vatCents),
			GrossStr:   formatCents(gross),
		})
	}
	return breakdown
}

func buildDocument(req InvoiceRequest, p *Profile, cfg docConfig, designKey, docType string) (pdfPath string, cleanup func(), err error) {
	noop := func() {}

	tmpDir, err := os.MkdirTemp("", "rechnung-*")
	if err != nil {
		return "", noop, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tmpDir) }

	// Copy every file the design ships, so designs can include arbitrary
	// .sty/.def/.tex/.png assets without registering them anywhere.
	// design.json is metadata and gets excluded; other docTypes' mainTex
	// files are also excluded to keep the build dir lean and avoid
	// surprising \input chains.
	designDir := "designs/" + designKey + "/"
	entries, err := embedded.ReadDir(strings.TrimSuffix(designDir, "/"))
	if err != nil {
		return "", cleanup, fmt.Errorf("read design dir %s: %w", designDir, err)
	}
	excluded := otherDocMainTeXNames(docType)
	excluded["design.json"] = true
	for _, e := range entries {
		if e.IsDir() || excluded[e.Name()] {
			continue
		}
		data, err := embedded.ReadFile(designDir + e.Name())
		if err != nil {
			return "", cleanup, fmt.Errorf("read embedded %s%s: %w", designDir, e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, e.Name()), data, 0644); err != nil {
			return "", cleanup, fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}
	// The mainTex must exist; surface a clearer error if the design forgot it.
	if _, err := os.Stat(filepath.Join(tmpDir, cfg.mainTexName)); err != nil {
		return "", cleanup, fmt.Errorf("design %q is missing required %s", designKey, cfg.mainTexName)
	}

	// Copy the profile's logo into the temp dir as logo.png. Missing logo
	// is non-fatal: designs can branch on \ifHasLogo and render the
	// profile.shortName wordmark as fallback. Designs that hard-require
	// the file (e.g. classic uses \includegraphics{logo.png} unconditionally)
	// will fail at LaTeX-build with a missing-file error — preferred over a
	// 500 at the HTTP layer because the LaTeX log shows which design needs it.
	hasLogo := false
	if p.Logo != "" {
		logoData, lerr := os.ReadFile(filepath.Join("logos", p.Logo))
		if lerr == nil {
			if werr := os.WriteFile(filepath.Join(tmpDir, "logo.png"), logoData, 0644); werr != nil {
				return "", cleanup, fmt.Errorf("write logo: %w", werr)
			}
			hasLogo = true
		} else {
			log.Printf("logo %q for profile %q not found; falling back to shortName wordmark", p.Logo, req.ProfileKey)
		}
	}

	// Build SenderCompany from lines joined with LaTeX newline.
	companyLines := make([]string, len(p.SenderCompanyLines))
	for i, l := range p.SenderCompanyLines {
		companyLines[i] = latexEscape(l)
	}

	// Escape line item descriptions; append VAT label when applicable.
	escapedItems := make([]LineItem, len(req.Items))
	for i, item := range req.Items {
		esc := LineItem{
			Description: itemDescription(item, req.UseVat),
			UnitPrice:   item.UnitPrice,
			Quantity:    item.Quantity,
			VatRate:     item.VatRate,
		}
		// Per-line discount display: compute the discount amount against the
		// undiscounted line net so the invoice shows full price minus Nachlass.
		lineNetCents := parseCents(item.UnitPrice) * parseCents(item.Quantity) / 100
		if dc := lineDiscountCents(item, lineNetCents); dc > 0 {
			esc.HasDiscount = true
			esc.DiscountAmountStr = formatCents(dc)
			if item.DiscountKind == "percent" {
				// Pass just the (escaped) number; each design's \FeeDiscount
				// macro appends the "%" sign so the percent rate is shown.
				esc.DiscountLabel = latexEscape(strings.TrimSpace(item.DiscountValue))
			}
		}
		escapedItems[i] = esc
	}

	// Compute VAT breakdown using original (unescaped) items.
	vatBreakdown := computeVatBreakdown(req.Items, req.UseVat)
	log.Printf("VAT breakdown for %s: %+v", req.InvoiceReference, vatBreakdown)

	// Aggregate totals.
	var totalNetCents, totalVatCents int64
	nonZeroRates := map[int]struct{}{}
	for _, b := range vatBreakdown {
		totalNetCents += b.NetCents
		totalVatCents += b.VatCents
		if b.Rate > 0 {
			nonZeroRates[b.Rate] = struct{}{}
		}
	}
	totalGrossCents := totalNetCents + totalVatCents
	hasAnyVat := req.UseVat && len(nonZeroRates) > 0
	hasMultipleVatRates := len(nonZeroRates) > 1

	// Resolve payment mode; default to "transfer" when omitted for backwards-compat.
	paymentMode := req.PaymentMode
	if paymentMode == "" {
		paymentMode = "transfer"
	}

	// Resolve clause flags.
	clauseSet := map[string]bool{}
	for _, c := range req.Clauses {
		clauseSet[c] = true
	}

	// Render a real EPC/Girocode QR into the build dir when the request wants
	// the QR (ShowQR), payment is by bank transfer (a SEPA QR is meaningless
	// for cash), and the profile has the data to build a valid payload. On any
	// failure we leave HasQRFile false so the design falls back to its
	// placeholder rather than failing the build.
	hasQRFile := false
	if !req.HideQR && paymentMode == "transfer" {
		if payload := buildEPCPayload(p, req, formatCents(totalGrossCents)); payload != "" {
			// Error correction level M is mandated by the EPC specification.
			if err := qrcode.WriteFile(payload, qrcode.Medium, 512, filepath.Join(tmpDir, "epc-qr.png")); err != nil {
				log.Printf("EPC QR render failed for %s: %v; falling back to placeholder", req.InvoiceReference, err)
			} else {
				hasQRFile = true
			}
		}
	}

	data := TemplateData{
		TaxID:             latexEscape(p.TaxID),
		SenderName:        latexEscape(p.SenderName),
		SenderCompany:     strings.Join(companyLines, `\\`),
		SenderStreet:      latexEscape(p.SenderStreet),
		SenderZIP:         latexEscape(p.SenderZIP),
		SenderCity:        latexEscape(p.SenderCity),
		SenderTelephone:   latexEscape(p.SenderTelephone),
		SenderMobilephone: latexEscape(p.SenderMobilephone),
		SenderEmail:       p.SenderEmail,
		SenderWeb:         p.SenderWeb,
		AccountRCPT:       latexEscape(p.SenderName),
		AccountIBAN:       latexEscape(p.AccountIBAN),
		AccountBIC:        latexEscape(p.AccountBIC),
		AccountBankName:   latexEscape(p.AccountBankName),
		VatID:             vatID(req, p),
		VatRate:           vatRate(req, p),
		InvoiceDate:       latexEscape(req.InvoiceDate),
		PayDate:           latexEscape(req.PayDate),
		InvoiceReference:  latexEscape(req.InvoiceReference),
		InvoiceSalutation: latexEscape(req.InvoiceSalutation),
		InvoiceText:       latexEscape(req.InvoiceText),
		InvoiceEnclosures: req.InvoiceEnclosures,
		InvoiceClosing:    latexEscape(req.InvoiceClosing),
		CustomerCompany:   latexEscape(req.CustomerCompany),
		CustomerName:      latexEscape(req.CustomerName),
		CustomerStreet:    latexEscape(req.CustomerStreet),
		CustomerZIP:       latexEscape(req.CustomerZIP),
		CustomerCity:      latexEscape(req.CustomerCity),
		ProjectTitle:      latexEscape(req.ProjectTitle),
		Items:             escapedItems,

		VatBreakdown:        vatBreakdown,
		NetTotalStr:         formatCents(totalNetCents),
		VatTotalStr:         formatCents(totalVatCents),
		GrossTotalStr:       formatCents(totalGrossCents),
		HasMultipleVatRates: hasMultipleVatRates,
		HasAnyVat:           hasAnyVat,

		ShortName: latexEscape(p.ShortName),
		HasLogo:   hasLogo,

		ShowQR:    !req.HideQR,
		HasQRFile: hasQRFile,
		IsEnglish: strings.EqualFold(req.Language, "en"),

		HasProjectTitle: req.ProjectTitle != "",

		PaymentMode:       paymentMode,
		IsPaymentTransfer: paymentMode == "transfer",
		IsPaymentCashDue:  paymentMode == "cash_due",
		IsPaymentCashPaid: paymentMode == "cash_paid",
		CashPaidDate:      latexEscape(req.CashPaidDate),

		ShowWarrantyExcluded: clauseSet["warranty_excluded"],
		ShowRetentionOfTitle: clauseSet["retention_of_title"],
		ShowLateFeeWarning:   clauseSet["late_fee_warning"],
	}

	if err := renderTemplate(tmpDir, "_data.tex", "templates/_data.tex.tmpl", data); err != nil {
		return "", cleanup, err
	}
	if err := renderTemplate(tmpDir, cfg.itemsOut, cfg.itemsTmpl, data); err != nil {
		return "", cleanup, err
	}

	// Run pdflatex twice so cross-references resolve correctly.
	// Only check the exit code on the final run — the first run commonly
	// returns exit 1 due to unresolved cross-references / missing .aux.
	for i := range 2 {
		cmd := exec.Command("pdflatex", "-interaction=nonstopmode", cfg.mainTexName)
		cmd.Dir = tmpDir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil && i == 1 {
			return "", cleanup, fmt.Errorf("pdflatex failed: %w\n\n%s", err, out.String())
		}
	}

	return filepath.Join(tmpDir, cfg.outputPDF), cleanup, nil
}

func renderTemplate(dir, outName, tmplPath string, data any) error {
	src, err := embedded.ReadFile(tmplPath)
	if err != nil {
		return fmt.Errorf("read template %s: %w", tmplPath, err)
	}
	tmpl, err := template.New(outName).Parse(string(src))
	if err != nil {
		return fmt.Errorf("parse template %s: %w", tmplPath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template %s: %w", tmplPath, err)
	}
	return os.WriteFile(filepath.Join(dir, outName), buf.Bytes(), 0644)
}

func vatID(req InvoiceRequest, p *Profile) string {
	if !req.UseVat {
		return ""
	}
	return p.VatID
}

// vatRate always returns 0 — VAT is now expressed per item in the description,
// not as a global rate on the invoice environment.
func vatRate(_ InvoiceRequest, _ *Profile) int {
	return 0
}

// buildEPCPayload assembles the EPC069-12 ("Girocode") version 002 payload for
// a SEPA Credit Transfer (SCT) from raw, un-escaped profile/request data. The
// returned string is encoded verbatim into the QR image; banking apps parse it
// to pre-fill a transfer. Fields are LF-separated; trailing empty fields are
// dropped. Returns "" (caller skips the QR) when IBAN or amount is missing.
//
// Field order (EPC069-12 §2.2): ServiceTag, Version, Charset, Identification,
// BIC, Name, IBAN, Amount, PurposeCode, StructuredRef, UnstructuredRemittance.
// We use the unstructured remittance (field 11) and leave the structured
// reference (field 10) empty, as the two are mutually exclusive.
func buildEPCPayload(p *Profile, req InvoiceRequest, grossTotalStr string) string {
	iban := strings.ReplaceAll(p.AccountIBAN, " ", "")
	if iban == "" || grossTotalStr == "" || grossTotalStr == "0.00" {
		return ""
	}

	name := truncateRunes(strings.TrimSpace(p.SenderName), 70)
	remittance := strings.TrimSpace("Rechnung " + req.InvoiceReference + " " + p.SenderName)
	remittance = truncateRunes(remittance, 140)

	fields := []string{
		"BCD",                  // Service Tag
		"002",                  // Version
		"1",                    // Character set: 1 = UTF-8
		"SCT",                  // Identification: SEPA Credit Transfer
		strings.TrimSpace(p.AccountBIC), // BIC (optional within SEPA)
		name,                            // Beneficiary name (≤70)
		iban,                            // IBAN, no spaces
		"EUR" + grossTotalStr,           // Amount, e.g. EUR398.50
		"",                              // Purpose code (unused)
		"",                              // Structured reference (unused)
		remittance,                      // Unstructured remittance (≤140)
	}

	// Drop trailing empty fields per spec to keep the payload compact.
	end := len(fields)
	for end > 0 && fields[end-1] == "" {
		end--
	}
	return strings.Join(fields[:end], "\n")
}

// truncateRunes shortens s to at most n runes (not bytes), keeping multibyte
// characters intact.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// itemDescription returns the LaTeX-escaped item description, with the VAT
// rate appended (e.g. "Cappuccino (19\% MwSt.)") when VAT is enabled.
func itemDescription(item LineItem, useVat bool) string {
	desc := latexEscape(item.Description)
	if !useVat || item.VatRate == "" {
		return desc
	}
	return desc + ` (` + item.VatRate + `\% MwSt.)`
}

// latexEscape escapes characters that are special in LaTeX text mode.
// It intentionally does not escape backslash, braces, tilde, or caret
// so users can include basic LaTeX in text fields if needed.
// SenderEmail and SenderWeb are intentionally not passed through this
// function — they are used inside \href / \url which handle their own escaping.
func latexEscape(s string) string {
	s = strings.ReplaceAll(s, `&`, `\&`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	s = strings.ReplaceAll(s, `#`, `\#`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
