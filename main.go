package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed static all:templates all:latex
var embedded embed.FS

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
	VatID              string   `json:"vatID"`
	VatRate            int      `json:"vatRate"` // optional; defaults to 19 when VatID is set
}

type LineItem struct {
	Description string `json:"description"`
	UnitPrice   string `json:"unitPrice"`
	Quantity    string `json:"quantity"`
}

type InvoiceRequest struct {
	ProfileKey        string     `json:"profileKey"`
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
}

var profiles = map[string]*Profile{}

func main() {
	if err := loadProfiles("profiles"); err != nil {
		log.Printf("warning: could not load profiles: %v", err)
	}

	mux := http.NewServeMux()

	staticFS, _ := fs.Sub(embedded, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /api/profiles", handleProfiles)
	mux.HandleFunc("GET /api/profiles/{key}", handleProfile)
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

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	handleDoc(w, r, "invoice")
}

func handleLieferschein(w http.ResponseWriter, r *http.Request) {
	handleDoc(w, r, "lieferschein")
}

type docConfig struct {
	mainTex    string
	itemsTmpl  string
	itemsOut   string
	extraFiles []string
	outputPDF  string
	filePrefix string
}

var docConfigs = map[string]docConfig{
	"invoice": {
		mainTex:    "_main.tex",
		itemsTmpl:  "templates/_invoice.tex.tmpl",
		itemsOut:   "_invoice.tex",
		extraFiles: []string{"invoice.sty", "invoice.def"},
		outputPDF:  "_main.pdf",
		filePrefix: "rechnung",
	},
	"lieferschein": {
		mainTex:    "_lieferschein_main.tex",
		itemsTmpl:  "templates/_lieferschein_items.tex.tmpl",
		itemsOut:   "_lieferschein_items.tex",
		extraFiles: []string{},
		outputPDF:  "_lieferschein_main.pdf",
		filePrefix: "lieferschein",
	},
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

	cfg := docConfigs[docType]
	pdfPath, cleanup, err := buildDocument(req, p, cfg)
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

func buildDocument(req InvoiceRequest, p *Profile, cfg docConfig) (pdfPath string, cleanup func(), err error) {
	noop := func() {}

	tmpDir, err := os.MkdirTemp("", "rechnung-*")
	if err != nil {
		return "", noop, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tmpDir) }

	// Copy embedded LaTeX files into temp dir.
	filesToCopy := append([]string{cfg.mainTex}, cfg.extraFiles...)
	for _, f := range filesToCopy {
		data, err := embedded.ReadFile("latex/" + f)
		if err != nil {
			return "", cleanup, fmt.Errorf("read embedded %s: %w", f, err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, f), data, 0644); err != nil {
			return "", cleanup, fmt.Errorf("write %s: %w", f, err)
		}
	}

	// Copy the profile's logo into the temp dir as logo.png.
	logoData, err := os.ReadFile(filepath.Join("logos", p.Logo))
	if err != nil {
		return "", cleanup, fmt.Errorf("read logo %s: %w", p.Logo, err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "logo.png"), logoData, 0644); err != nil {
		return "", cleanup, fmt.Errorf("write logo: %w", err)
	}

	// Build SenderCompany from lines joined with LaTeX tabular separator.
	companyLines := make([]string, len(p.SenderCompanyLines))
	for i, l := range p.SenderCompanyLines {
		companyLines[i] = latexEscape(l)
	}

	// Escape line item descriptions; leave prices/quantities as-is (numeric).
	escapedItems := make([]LineItem, len(req.Items))
	for i, item := range req.Items {
		escapedItems[i] = LineItem{
			Description: latexEscape(item.Description),
			UnitPrice:   item.UnitPrice,
			Quantity:    item.Quantity,
		}
	}

	data := TemplateData{
		TaxID:             latexEscape(p.TaxID),
		SenderName:        latexEscape(p.SenderName),
		SenderCompany:     strings.Join(companyLines, `\\&`),
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
	}

	if err := renderTemplate(tmpDir, "_data.tex", "templates/_data.tex.tmpl", data); err != nil {
		return "", cleanup, err
	}
	if err := renderTemplate(tmpDir, cfg.itemsOut, cfg.itemsTmpl, data); err != nil {
		return "", cleanup, err
	}

	// Run pdflatex twice so cross-references resolve correctly.
	for i := range 2 {
		cmd := exec.Command("pdflatex", "-interaction=nonstopmode", cfg.mainTex)
		cmd.Dir = tmpDir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			return "", cleanup, fmt.Errorf("pdflatex run %d failed: %w\n\n%s", i+1, err, out.String())
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

func vatRate(req InvoiceRequest, p *Profile) int {
	if !req.UseVat {
		return 0
	}
	if p.VatRate > 0 {
		return p.VatRate
	}
	return 19
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
