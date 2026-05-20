# Sample-PDFs

Generiert mit Mock-Daten zum Vergleich der Designs und Szenarien.

## Übersicht (15 PDFs — alle 6 Profile × beide Designs)

| Datei | Design | Profil | Logo-Status | Szenario |
|---|---|---|---|---|
| `classic-rico-solo-kleinunternehmer.pdf` | classic | rico-solo | Logo (`logo.png`) | Kleinunternehmer §19, 2 Items |
| `classic-rico-catering-kleinunternehmer.pdf` | classic | rico-catering | Logo (`logo.png`) | Kleinunternehmer §19, 3 Items |
| `classic-milky-kleinunternehmer.pdf` | classic | milky | Logo (`milky-logo.png`) | Kleinunternehmer §19, 4 Items |
| `classic-snacks-kleinunternehmer.pdf` | classic | snacks | shortName-Fallback („snacks") | Kleinunternehmer §19, 2 Items |
| `classic-rico-kleinstadt-ust-19.pdf` | classic | rico-kleinstadt | Logo (`kleinstadt_logo.png`) | USt-pflichtig, Single-Rate 19% |
| `classic-rico-kleinstadt-mischsatz.pdf` | classic | rico-kleinstadt | Logo | USt-pflichtig, Mischsatz 7%+19% |
| `classic-frameway-ust-19.pdf` | classic | frameway | shortName-Fallback („frameway") | USt-pflichtig, Single-Rate 19% |
| `classic-rico-kleinstadt-lieferschein.pdf` | classic | rico-kleinstadt | Logo | Lieferschein (modern unterstützt das nicht) |
| `modern-rico-solo-kleinunternehmer.pdf` | modern | rico-solo | Logo | Kleinunternehmer §19 |
| `modern-rico-catering-kleinunternehmer.pdf` | modern | rico-catering | Logo | Kleinunternehmer §19 |
| `modern-milky-kleinunternehmer.pdf` | modern | milky | Logo | Kleinunternehmer §19 |
| `modern-snacks-kleinunternehmer.pdf` | modern | snacks | shortName-Fallback | Kleinunternehmer §19 |
| `modern-rico-kleinstadt-ust-19.pdf` | modern | rico-kleinstadt | Logo | USt-pflichtig, Single-Rate 19% |
| `modern-rico-kleinstadt-mischsatz.pdf` | modern | rico-kleinstadt | Logo | USt-pflichtig, Mischsatz 7%+19% |
| `modern-frameway-ust-19.pdf` | modern | frameway | shortName-Fallback | USt-pflichtig, Single-Rate 19% |

## Branding-Logik

Beide Designs verwenden eine einheitliche Branding-Regel:

```latex
\ifHasLogo
  \includegraphics{logo.png}   % wenn Logo-Datei in logos/ existiert
\else
  \textbf{\shortName}           % vordefinierter Spitzname aus Profil
\fi
```

`shortName` ist ein neues Profil-Feld (z.B. `"shortName": "frameway"`). Es wird nur dann gerendert, wenn die im Profil referenzierte Logo-Datei in `logos/` nicht gefunden wurde.

**Bei modern** kommt der vollständige Firmenname (`senderCompanyLines` → `\senderCompany`) klein direkt vor die Adresse — sowohl im Logo- als auch im shortName-Fall.

## Mock-Daten

Test-Kunden in Deutschland (München, Berlin, Lindau, Stuttgart, Kempten, Wangen, Friedrichshain).
Datum: 20.05.2026, Fälligkeit: 03.06. bzw. 10.06.2026.
Rechnungsnummern: profilspezifische Prefixe (KS-, FW-, CAT-, MC-, SN-).

## Re-Generieren

```bash
go run main.go &
# Curl-Aufrufe siehe Git-Historie der samples-Commits
```

## Bekannte Daten-Gaps (nicht durch Samples verursacht)

Die Profile `frameway` und `snacks` referenzieren Logo-Dateien (`frameway-logo.png`, `snacks-logo.png`), die in `logos/` nicht existieren. Statt zu crashen, wird der `shortName`-Fallback gerendert. Sobald die Logo-Dateien ergänzt sind, übernimmt automatisch das Bild.
