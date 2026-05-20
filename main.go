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
	"text/template"
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

type LineItem struct {
	Description string `json:"description"`
	UnitPrice   string `json:"unitPrice"`
	Quantity    string `json:"quantity"`
	VatRate     string `json:"vatRate"` // "", "7", or "19"
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

func main() {
	if err := loadProfiles("profiles"); err != nil {
		log.Printf("warning: could not load profiles: %v", err)
	}
	if err := loadDesigns("designs"); err != nil {
		log.Printf("warning: could not load designs: %v", err)
	}

	mux := http.NewServeMux()

	staticFS, _ := fs.Sub(embedded, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /api/profiles", handleProfiles)
	mux.HandleFunc("GET /api/profiles/{key}", handleProfile)
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
		escapedItems[i] = LineItem{
			Description: itemDescription(item, req.UseVat),
			UnitPrice:   item.UnitPrice,
			Quantity:    item.Quantity,
			VatRate:     item.VatRate,
		}
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
