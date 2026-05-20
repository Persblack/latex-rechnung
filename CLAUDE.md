# CLAUDE.md

## Rollen

Du operierst in einer von neun Rollen. Der User sagt, welche. Bleib in der Rolle, bis er sie explizit wechselt. Lies die zugehörige Datei beim Aktivieren.

| Rolle | Agent-Datei | Wofür |
|---|---|---|
| **Orchestrator** | [`ORCHESTRATOR.md`](ORCHESTRATOR.md) | Koordination, Routing, `INVOICE_STATE.md` pflegen, Branch-Management |
| **LaTeX Engineer** | [`.claude/agents/latex-engineer.md`](.claude/agents/latex-engineer.md) | Build, Pakete, Engine, `.sty`/`.def`-Architektur — Umbrella-Agent |
| **Typographer** | [`.claude/agents/typographer.md`](.claude/agents/typographer.md) | Layout, Schrift, KOMA-Script, Mikrotypografie — Umbrella-Agent |
| **Editor** | [`.claude/agents/editor.md`](.claude/agents/editor.md) | Korrekturlesen, Stil, deutsche Sprache — Umbrella-Agent |
| **Bibliographer** | [`.claude/agents/bibliographer.md`](.claude/agents/bibliographer.md) | BibTeX/BibLaTeX — Umbrella-Agent (für dieses Projekt selten relevant) |
| **Invoice Designer** | [`.claude/agents/invoice-designer.md`](.claude/agents/invoice-designer.md) | Rechnungs-Layout-Designs in `designs/<name>/` — projektspezifisch |
| **Bookkeeper** | [`.claude/agents/bookkeeper.md`](.claude/agents/bookkeeper.md) | Positionen, Summen, Skonti, MwSt-Anwendung pro Item — projektspezifisch |
| **Legal Compliance** | [`.claude/agents/legal-compliance.md`](.claude/agents/legal-compliance.md) | §14 UStG Pflichtangaben, §19 UStG Kleinunternehmer, USt-ID-Logik — projektspezifisch |
| **Client Communicator** | [`.claude/agents/client-communicator.md`](.claude/agents/client-communicator.md) | Anrede, Rechnungstext, Höflichkeitsformeln, deutscher Geschäftsbriefstil — projektspezifisch |

Beim Aktivieren einer Rolle: die Agent-Datei lesen, `INVOICE_STATE.md` lesen, dann unter den Rollen-Instruktionen weiterarbeiten.

## Agent-Setup

Subagent-Definitionen liegen in zwei Quellen:
- **Umbrella** (`../agents/`): projektübergreifend, gepflegt im Umbrella-Verzeichnis. In `.claude/agents/` per **Symlink** eingebunden.
- **Projektspezifisch** (`agents/`): nur für dieses Repo. Auch in `.claude/agents/` per Symlink (damit alle Agent-Quellen sichtbar in `agents/` liegen und `.claude/agents/` nur Discovery-Mechanismus ist).

Claude Code findet Agents automatisch in `.claude/agents/`.

**Warum der Orchestrator kein Subagent ist:** Subagents können in Claude Code keine weiteren Subagents spawnen. Da der Orchestrator genau das tut, läuft er als Main-Session und liest `ORCHESTRATOR.md` direkt.

### Modell-Tiers

| Tier | Modell | Genutzt für | Warum |
|---|---|---|---|
| **Kritisches Urteil** | Opus | LaTeX Engineer, Legal Compliance | Strukturelle Architektur-Entscheidungen (Engineer); rechtliche Haftungsrisiken (Compliance) |
| **Produktionsarbeit** | Sonnet | Typographer, Editor, Bibliographer, Invoice Designer, Bookkeeper, Client Communicator | Etablierte Verfahren, hohe Volumina, geringes Risiko |

---

## Autonomie

Vollautonomie ist erlaubt:
- Keine Bestätigung nötig für Branches, Commits, Web-Suchen, File-Operationen
- Bei Unsicherheit zwischen zwei gleichwertigen Wegen: einen wählen, kurz begründen
- Parallele Subagents nutzen, wenn Aufgaben unabhängig sind
- Web-Suchen und Downloads sind erlaubt
- Git: volle Befugnis außer Force-Push auf `master`
- Harter Stopp nur bei irreversiblem Schaden (Repo löschen, Force-Push auf `master`)

---

## INVOICE_STATE.md

`INVOICE_STATE.md` ist die primäre Koordinationsdatei. Sie hält fest:
- Verfügbare Designs (`designs/*/`) und welches als Default gilt
- Verfügbare Profile (`profiles/*.json`)
- Letzte Rechnungs-Referenznummer pro Profil
- Offene Architektur-Entscheidungen
- Letzte Agent-Aktion (was, wer, welcher Branch)

**Jede Session muss `INVOICE_STATE.md` beim Start lesen.** Der Orchestrator updatet nach jeder signifikanten Aktion.

---

## Build

Go-Server starten:
```sh
go run main.go
```
Dashboard: `http://localhost:8080`.

Direkter LaTeX-Build (für isolierte Tests eines Designs):
```sh
cd designs/<name> && pdflatex _main.tex
```
Zweimal laufen lassen für Cross-References. Output: `_main.pdf`.

## Architektur (Multi-Design)

Dieser Generator unterstützt mehrere visuell unterschiedliche Rechnungs-Designs, die **die gleichen Daten** (Profile, Kunden, Positionen) konsumieren.

```
latex-rechnung/
├── main.go                 # HTTP-Server, Profile-Loading, Design-Routing, pdflatex-Build
├── profiles/<key>.json     # Sender-Daten pro Unternehmen
├── designs/                # Layout-Varianten — pro Design ein Ordner
│   ├── classic/            # Default-Design (Oliver Corffs `invoice` pkg + KOMA scrlttr2)
│   │   ├── _main.tex
│   │   ├── _lieferschein_main.tex
│   │   ├── invoice.sty
│   │   ├── invoice.def
│   │   └── design.json     # {name, description, supports: ["invoice","lieferschein"]}
│   └── <weitere>/          # weitere Designs
├── templates/              # SHARED Datenrender (Go text/template)
│   ├── _data.tex.tmpl      # injiziert Sender/Kunde/Datum/IBAN/…
│   ├── _invoice.tex.tmpl   # injiziert Positionen
│   └── _lieferschein_items.tex.tmpl
├── logos/                  # Logo-PNGs pro Profil
└── static/index.html       # Web-Dashboard mit Profil- + Design-Selector
```

**Sharing-Modell:**
- Daten kommen aus `profiles/` + Request-Payload (Kunde, Positionen).
- `templates/` rendern Daten in `.tex`-Snippets, die alle Designs verwenden.
- `designs/<name>/_main.tex` lädt die gerenderten Snippets via `\input{_data.tex}` und `\input{_invoice.tex}` und definiert das **Layout**.

**Override-Pattern (optional):** Falls ein Design ein eigenes Template braucht, kann es `designs/<name>/templates/<file>.tmpl` definieren. Das überschreibt die Shared-Variante für nur dieses Design.

## Workflow für eine neue Rechnung

1. Im Dashboard Unternehmen (Profil) und Design wählen
2. Kundenadresse, Datum, Rechnungsnummer, Text, Positionen eingeben
3. „Generieren" → PDF-Download

## Workflow für ein neues Design

1. Branch: `invoice-designer/<design-name>`
2. Ordner anlegen: `designs/<name>/` mit `design.json`
3. `_main.tex` schreiben (kompiliert isoliert via `pdflatex` mit Test-Daten)
4. `_lieferschein_main.tex` schreiben falls Lieferscheine unterstützt
5. Test über Dashboard mit realem Profil
6. PR, Merge nach `master`

## Git

Commit-Message: **das Warum, nicht das Was**.

Format: `<role>: <was+warum>`

Beispiel: `invoice-designer: add modern design — tech-minimal aesthetic per inspo/invoice-light reference`
