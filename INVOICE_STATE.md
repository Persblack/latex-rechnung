# INVOICE_STATE.md

Stand: 2026-05-20 (nach Orchestrator-Session)

## Designs

| Key | Status | Beschreibung | Pfad |
|---|---|---|---|
| `classic` | **produktiv** | KOMA `scrlttr2` + Oliver Corffs `invoice`-Paket. Pflichtangaben-Block in Anschriftentabelle (Steuernr., USt-ID, IBAN, BIC, Rechnungsnr., Leistungsdatum). MwSt-Aufschlüsselung als Folge-Tabelle wenn `\ifHasAnyVat`. | `designs/classic/` |
| `modern` | **produktiv (mit Polish-Notes)** | Tech-minimal Layout nach `inspo/invoice-light.jpeg` — Monospace, blaue Akzente, Crop-Marks, vertikale Labels (`CLIENT`/`SYSTEM ID`/`PAYMENT NODE`), **echter EPC/Girocode-QR** (server-seitig in Go gerendert, `\ifHasQRFile`; TikZ-Platzhalter nur noch Fallback). Dynamischer Total-Block mit Per-Rate-Aufschlüsselung. | `designs/modern/` |

**Default-Design im Dashboard:** `classic`.

## Profile

| Key | Unternehmen | USt-Status |
|---|---|---|
| `rico-solo` | Rico Klatte – Einzelunternehmen | Kleinunternehmer §19 |
| `rico-kleinstadt` | Kleinstadt Roastery GbR | USt-pflichtig (vatID gesetzt) |
| `rico-catering` | Rico Klatte – Catering GbR | Kleinunternehmer (impliziert, `vatID` leer) — **mit Steuerberater abgleichen** |
| `frameway` | Frameway GbR | USt-pflichtig |
| `milky` | Milky Cream GbR | Kleinunternehmer (impliziert) — **mit Steuerberater abgleichen** |
| `snacks` | Kleinstadt Snacks GbR | Kleinunternehmer (impliziert) — **mit Steuerberater abgleichen** |

(siehe `profiles/*.json` für vollständige Daten — `vatID` leer = System behandelt als Kleinunternehmer)

## Compliance-Status (Stand nach Phase B)

Audit-Dokument: [`docs/compliance/audit-2026-05-20.md`](docs/compliance/audit-2026-05-20.md)

| Pflicht (§14 UStG) | classic | modern |
|---|---|---|
| Nr.1 Absender komplett | ✓ | ✓ |
| Nr.2 Empfänger komplett (DIN 5008) | ✓ | ✓ |
| Nr.3 Steuernr. / USt-ID | ✓ | ✓ |
| Nr.4 Ausstellungsdatum | ✓ | ✓ |
| Nr.5 fortlaufende Rechnungsnr. | ✓ | ✓ |
| Nr.6 Menge + Art Leistung | ✓ | ✓ |
| Nr.6 Leistungsdatum | ✓ (Hinweis „entspricht Rechnungsdatum") | ✓ (im Metablock) |
| Nr.8 Entgelt nach Steuersätzen aufgeschlüsselt | ✓ (Aufschlüsselungs-Tabelle) | ✓ (Per-Rate im Total-Block) |
| Nr.9 Steuersatz + Steuerbetrag ODER §19-Hinweis | ✓ | ✓ |
| §19-Disclaimer-Wortlaut | „Im ausgewiesenen Betrag ist gemäß § 19 UStG keine Umsatzsteuer enthalten." | „Gemäß § 19 UStG wird keine Umsatzsteuer berechnet." |

**Rechtliche Bewertung:** beide Designs sind **produktionstauglich für Kleinunternehmer- und USt-pflichtige Rechnungen** (Single- und Mischsatz). Audit hat keinen verbliebenen Blocker.

## Architektur-Highlights (was wo lebt)

- **Daten** (Profile, Kunde, Items, Datum, Anrede): `profiles/*.json` + Request-Payload.
- **MwSt-Berechnung**: `main.go::computeVatBreakdown` — Fixed-Point in Cents, kaufmännische Rundung, Aggregation pro Steuersatz.
- **Daten-Snippets** (gemeinsam für alle Designs): `templates/_data.tex.tmpl` und `templates/_invoice.tex.tmpl`.
- **Design-Macros** (von Designs überschreibbar): `\VatBreakdownRow{rate}{net}{vat}{gross}` + `\ifHasAnyVat` + `\NetTotal` / `\VatTotal` / `\GrossTotal`.
- **Design-spezifischer LaTeX-Code**: `designs/<key>/_main.tex` (Pflicht), optional eigene `*.sty`/`*.def`, optional `_lieferschein_main.tex`.
- **HTTP-Routing**: `main.go` mit `GET /api/designs`, `GET /api/designs/{key}`, `POST /generate?design=...`, `POST /lieferschein?design=...`. Backwards-Kompat: leeres `design`-Feld → `classic`.
- **Frontend**: `static/index.html` mit Design-Dropdown neben Profil-Dropdown.

## Offene Themen (Polish, nicht-Blocker)

1. **Modern Big-Type-Firmenname**: bei Profilen mit 3+ Zeilen Firmenname (z.B. `rico-kleinstadt`: „RK Holding GmbH / und Rico Klatte / Kleinstadt Roastery GbR") wird die `\fontsize{40pt}`-Wortmarke übergroß (5 Zeilen Big-Type). Fix-Optionen: (a) auto-shrink via `\scalebox`, (b) separater `\brandName`-Profile-Feld einführen, (c) erste Zeile nur als Big-Type.
2. **Classic doppelte Summenzeile**: Oliver Corffs invoice.sty rendert „Gesamtsumme 350.00" (= Netto-Summe, weil global `\vatRate=0`), direkt darüber der Block „Aufschlüsselung … Summe Brutto 398.50". Verwirrend. Fix-Vorschlag: invoice.sty-`\Gesamtsumme`-Label patchen auf „Netto-Summe" wenn `\ifHasAnyVat`, oder die invoice.sty-Summenzeile unterdrücken und stattdessen die Aufschlüsselungstabelle als primären Total nehmen.
3. **`vatID`-Check der „impliziten" Kleinunternehmer-Profile** (rico-catering, milky, snacks): mit Steuerberater verifizieren — falls eines davon eigentlich USt-pflichtig ist, `vatID` ergänzen, sonst Compliance-Risiko (§14c Abs.2 bei nicht ausgewiesener Steuer).
4. **Override-Pattern für Templates** (`designs/<name>/templates/`): noch nicht implementiert, kein aktuelles Design braucht es.

## Letzte Aktionen (Orchestrator-Session 2026-05-20)

1. ✅ Agent-Team initialisiert (8 Agents, Umbrella + projektspezifisch via Symlinks) — PR #1
2. ✅ Multi-Design-Architektur (`main.go` Routing, `/api/designs`, Frontend-Dropdown) — PR #2 (`latex-engineer/multi-design-routing`)
3. ✅ Modern Design v1 — PR #3 (`invoice-designer/modern`)
4. ✅ Hotfix: extraFiles auto-discovery aus designs/<key>/ — PR #4 (`latex-engineer/per-design-extra-files`)
5. ✅ Modern Bug-Fixes: Item-Render + Inspo-Placeholder — PR #5 (`invoice-designer/modern-fixes`)
6. ✅ Compliance-Audit beider Designs — PR #6 (`legal-compliance/audit-2026-05-20`)
7. ✅ VAT-Breakdown-Datenschicht (`computeVatBreakdown`, Template-Macros) — direct merge (`bookkeeper/vat-breakdown-data-layer`)
8. ✅ Compliance-UI-Fixes (LR-mode, §19-Wortlaut, MWSt-0%-Suppress, Leistungsdatum, Layout) — direct merge (`invoice-designer/compliance-ui-fixes`)
9. ✅ VAT-Breakdown-UI-Integration in beide Designs — direct merge (`invoice-designer/vat-breakdown-ui`)
10. ✅ INVOICE_STATE.md aktualisiert (diese Aktualisierung).
11. ✅ Echter EPC/Girocode-QR im modern-Design (Go `buildEPCPayload` + `skip2/go-qrcode`, `\ifHasQRFile`-Toggle; per zbarimg verifiziert) — Branch `invoice-designer/epc-qr` (Commit `b3d2385`)
12. ✅ Editierbare Rechnungs-Persistenz: `invoices/<nr>.json` speichert die komplette `InvoiceRequest`; Endpunkte `GET/POST /api/invoices`, `GET/PUT/DELETE /api/invoices/{key}` (POST = neu, 409 bei Kollision; PUT = Überschreiben). Dashboard-Dropdown zum Laden/Speichern/Löschen. Spiegelt das `recipients/`-Muster. — Commit `2f5c1b3`
13. ✅ Zwei Remexian-Rechnungen angelegt (`invoices/2026-001.json` Produktkatalog, `invoices/2026-002.json` Dashboard/Metabase; modern, rico-solo §19). Wert-mit-Nachlass: Listenpreis 1.200/900 € mit Festbetrag-Nachlass auf je 399 € netto, Hosting Jahrespauschale 12× mit Nachlass; Gesamt 518,88 / 638,88 €. Modern-Layout für 1-Seiten-Build verdichtet. — Commits `f456fff`, `4f11ccc`, `c824de6`
14. ✅ Konfigurierbarer Dezimaltrenner (`\fmtmoney`-Makro, Go-Feld `decimalSeparator`, Dashboard-Toggle; Default Komma für EUR). Anzeige-only — FP + EPC-QR bleiben Punkt. — Commit `915f3bc`
15. ✅ Alles nach `master` gemergt (Fast-Forward), Feature-Branches gelöscht. Nach `origin/master` gepusht.
16. ✅ Optionale Positions-Einheit (`LineItem.unit`, z.B. „Std.") — durchgereicht als 4. `\Fee`-Argument in beiden Designs (classic: Corffs `\Fee`/`\Fee@Line` auf 4-arg gepatcht); Dashboard-Einheit-Feld pro Zeile. Plus Tausender-Gruppierung in `\fmtmoney` (DE 14.910,00), auch in classic (`\Print@Value`). — Commit `2c1cc72`
17. ✅ Neues Profil `selim-oezdogan` (Einzelperson, USt-pflichtig) mit **Platzhaltern** (Steuernr./USt-IdNr./IBAN/BIC/Bank „ergänzen" — noch nicht versandfertig). Rechnung `invoices/so-2026-001.json` (Selim → Benjamin Wojtowicz, classic, 19% MwSt, Netto 14.910 €) aus „Rechnung S an B.pdf" übernommen + Korrekturen. — Commit `aa3f5aa`
