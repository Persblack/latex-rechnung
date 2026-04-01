# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Git

Always commit message the why not the what

## Build

Compile the invoice PDF with:
```sh
pdflatex _main.tex
```

Run twice if references need to be resolved. The output is `_main.pdf`.

## Architecture

This is a LaTeX invoice template for freelance/small-business invoicing under the German Kleinunternehmerregelung (§ 19 UStG — no VAT).

**File roles:**

- `_main.tex` — Document root. Sets up the KOMA-Script letter (`scrlttr2`), imports `_data.tex`, and renders the invoice using the `invoice` package. The VAT disclaimer and signature block live here.
- `_data.tex` — All configurable data: invoice metadata (`\invoiceDate`, `\invoiceReference`, `\invoiceSalutation`, `\invoiceText`), customer address (`\customerName`, etc.), sender/company details, and bank account info.
- `_invoice.tex` — Line items only. Uses `\ProjectTitle{}`, `\Fee{description}{unit_price}{quantity}`, `\Discount{label}{percent}`, and `\EBCi{label}{amount}` for expenses. The total is calculated automatically.
- `invoice.sty` / `invoice.def` — Third-party package (Oliver Corff, v0.9). `invoice.def` contains localizable labels and is the only file in the package intended for modification (e.g., to add language support).
- `logo.png` — Replace with your own logo; referenced in `_main.tex` at 20% text width.

**Workflow for a new invoice:**

1. Update `_data.tex` with the new invoice date, reference number, customer, and invoice text.
2. Update `_invoice.tex` with the new line items.
3. Run `pdflatex _main.tex`.
