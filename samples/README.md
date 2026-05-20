# Sample-PDFs

Generiert mit Mock-Daten zum Vergleich der Designs und Szenarien.

## Übersicht

| Datei | Design | Profil | Szenario |
|---|---|---|---|
| `classic-rico-solo-kleinunternehmer.pdf` | classic | rico-solo | Kleinunternehmer §19, 2 Items |
| `classic-rico-catering-kleinunternehmer.pdf` | classic | rico-catering | Kleinunternehmer §19, 3 Items (Catering-Pauschale) |
| `classic-milky-kleinunternehmer.pdf` | classic | milky | Kleinunternehmer §19, 4 Items (Eis-Lieferung) |
| `classic-rico-kleinstadt-ust-19.pdf` | classic | rico-kleinstadt | USt-pflichtig, Single-Rate 19%, 2 Items |
| `classic-rico-kleinstadt-mischsatz.pdf` | classic | rico-kleinstadt | USt-pflichtig, Mischsatz 7%+19%, 3 Items |
| `classic-rico-kleinstadt-lieferschein.pdf` | classic | rico-kleinstadt | Lieferschein (keine Preise/Summen) |
| `modern-rico-solo-kleinunternehmer.pdf` | modern | rico-solo | Kleinunternehmer §19 |
| `modern-rico-catering-kleinunternehmer.pdf` | modern | rico-catering | Kleinunternehmer §19 |
| `modern-milky-kleinunternehmer.pdf` | modern | milky | Kleinunternehmer §19 |
| `modern-rico-kleinstadt-ust-19.pdf` | modern | rico-kleinstadt | USt-pflichtig, Single-Rate 19% |
| `modern-rico-kleinstadt-mischsatz.pdf` | modern | rico-kleinstadt | USt-pflichtig, Mischsatz 7%+19% |

**Hinweis:** `modern` unterstützt nur Rechnungen, keinen Lieferschein. Siehe `designs/modern/design.json`.

## Mock-Daten

Alle Samples nutzen denselben Test-Kundensatz für Vergleichbarkeit:
- Adressen: München / Berlin / Kempten / Lindau / Wangen
- Datum: 20.05.2026, Fälligkeit: 03.06. bzw. 10.06.2026
- Rechnungsnummern: profilspezifische Prefixe (KS-, FW-, CAT-, MC-, …)

## Bekannte Datenfehler in den Profilen (nicht durch Samples verursacht)

- `profiles/frameway.json` referenziert `logos/frameway-logo.png` → **Datei fehlt** → 500 beim Generieren.
- `profiles/snacks.json` referenziert `logos/snacks-logo.png` → **Datei fehlt** → 500.

Fix-Optionen: (a) Logo-Dateien ergänzen, (b) Profile auf `logo.png` (Default) umstellen, (c) im Go-Code Fallback auf `logo.png` einbauen.

Diese beiden Profile sind in der Sample-Sammlung daher **nicht enthalten**.

## Re-Generieren

```bash
go run main.go &
# dann den entsprechenden curl-Aufruf aus der Orchestrator-Session — Befehle siehe Git-Historie
```
