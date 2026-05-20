# INVOICE_STATE.md

Stand: 2026-05-20

## Designs

| Key | Status | Beschreibung | Pfad |
|---|---|---|---|
| `classic` | produktiv (geplant) | Bestehende Layout-Variante mit Oliver Corffs `invoice` pkg + KOMA `scrlttr2`. Wird beim Multi-Design-Refactor aus dem Root in `designs/classic/` verschoben. | `designs/classic/` |
| `modern` | konzept | Tech-minimales Design nach `inspo/invoice-light.jpeg` — Monospace, blaue Akzente, Crop-Marks, vertikale Labels, QR/Barcode-Optik. Noch nicht implementiert. | `designs/modern/` |

**Default-Design:** `classic` (bis `modern` produktiv ist).

## Profile

| Key | Unternehmen | Notiz |
|---|---|---|
| `rico-solo` | Einzelgewerbe Rico Klatte | Kleinunternehmer §19 UStG |
| `rico-kleinstadt` | Kleinstadt Roastery | |
| `rico-catering` | Klatte Catering / Kaffee Klatsch | |
| `frameway` | Frameway | |
| `milky` | MilkyCream | |
| `snacks` | Snacks | |

(siehe `profiles/*.json` für vollständige Daten)

## Offene Architektur-Themen

1. **Multi-Design-Refactor in `main.go` ausstehend.** Aktuell hartcodiert auf `latex/_main.tex`. Bedarf: Routing per `design`-Key, `designs/<name>/_main.tex` als neue Source-of-Truth, Templates weiter shared aus `templates/`.
2. **Frontend-Dropdown für Design-Auswahl ausstehend** in `static/index.html`.
3. **`GET /api/designs`-Endpoint ausstehend.**
4. **Override-Pattern für Templates** (`designs/<name>/templates/`) noch nicht entschieden — erst implementieren, wenn ein Design es braucht.

## Offene Compliance-Themen

- Keine.

## Letzte Aktion

- **2026-05-20** — Orchestrator (Session-Start): Agent-Struktur initialisiert (Umbrella + Projekt). 8 Agent-Files in `.claude/agents/` via Symlinks verfügbar. Nächster Schritt: Multi-Design-Refactor und Konzept für `modern`.
