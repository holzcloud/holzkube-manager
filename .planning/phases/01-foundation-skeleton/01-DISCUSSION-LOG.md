# Phase 1: Foundation Skeleton - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-08-27
**Phase:** 1-foundation-skeleton
**Areas discussed:** Erstlauf & Erreichbarkeit, Sudo-Mode & Session-Politik, UI-Shell & Sprache, Audit-Log — Form & Sichtbarkeit

---

## Erstlauf & Erreichbarkeit

### Erstes Betreiber-Konto

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Setup-Wizard in der UI | Ohne Konto leitet jede Route auf `/setup`; Anlage im Browser, danach `/setup` tot. Kein Terminal nötig. Wer-zuerst-kommt-Lücke durch Bind auf `127.0.0.1` entschärft | ✓ |
| Bootstrap-Token auf stdout | Einmal-Token im Startlog, UI verlangt ihn vor der Konto-Anlage. Schließt die Lücke auch bei offenem Bind, kostet einen Terminal-Blick | |
| CLI-Subcommand | `holzkubed user create` mit interaktivem Passwort-Prompt. Am einfachsten zu bauen, aber Erstlauf ohne Terminal unmöglich | |
| Du entscheidest | — | |

**Wahl:** Setup-Wizard in der UI
**Notizen:** Folgt demselben Prinzip wie der Core Value — wenn ein Weg das Terminal erzwingt, ist er der falsche, auch wenn er technisch einfacher wäre.

### Datenverzeichnis

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| XDG: `~/.local/share/holzkube` | `$XDG_DATA_HOME` mit Fallback; `0700`/`0600`; funktioniert auf darwin und linux ohne root; Container zeigt per `--data-dir` auf ein Volume | ✓ |
| `~/.holzkube` | Dotdir wie `~/.talos` und `~/.kube` — vertraut für die Zielgruppe, ignoriert aber die XDG-Konvention | |
| `/var/lib/holzkube` | Klassischer Daemon-Pfad, passt zu systemd; braucht auf der macOS-Dev-Box root oder eine Sonderbehandlung | |
| Du entscheidest | — | |

**Wahl:** XDG mit Fallback `~/.local/share/holzkube`
**Notizen:** —

### Konfigurationsquelle

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Flags + ENV, keine Datei | `HOLZKUBE_*`-ENV je Flag, Präzedenz Flag > ENV > Default. Kein Parser, kein Suchpfad, nichts zu migrieren; Compose arbeitet nativ mit ENV | ✓ |
| Datei + ENV + Flags | Zusätzlich YAML/TOML im Datenverzeichnis als unterste Ebene. Bequem, kostet Suchpfad und Schema-Version | |
| Nur Flags | Radikal einfach, aber Geheimnisse landen in `ps` und in der Compose-Datei als Klartext | |
| Du entscheidest | — | |

**Wahl:** Flags + ENV, keine Datei
**Notizen:** Effektiver Wert jeder Option wird beim Start geloggt.

### TLS-Trust

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Fingerprint im Startlog | Langlebiges self-signed Zertifikat bei Erstlauf, SHA-256-Fingerprint geloggt, Warnung einmal akzeptiert. Null Zusatzcode | ✓ |
| Lokale CA erzeugen | Eigene Mini-CA, CA-Import in den System-Trust, danach grünes Schloss. Mehr Code, mehr Schlüsselmaterial, Import pro Rechner manuell | |
| Eigenes Zertifikat als Pflicht | Self-signed nur als Notnagel, dokumentierter Weg ist Reverse-Proxy oder Let's Encrypt. Verlagert die Arbeit in die Betriebsdoku | |
| Du entscheidest | — | |

**Wahl:** Fingerprint beim Start ausgeben, Warnung akzeptieren
**Notizen:** Bereitgestelltes Zertifikat wird weiterhin akzeptiert. `--insecure-http` bleibt auf Loopback beschränkt.

---

## Sudo-Mode & Session-Politik

### Dauer der Re-Authentifizierung

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Zeitfenster 5 Minuten | Wie `sudo`; Fenster bei jeder destruktiven Aktion neu gesetzt. Serien-Operationen bleiben erträglich, Schaden einer gestohlenen Session bleibt begrenzt | ✓ |
| Jede Aktion einzeln | Maximale Sicherheit, maximale Reibung — fünf Nodes, fünfmal tippen. Reibung führt erfahrungsgemäß zu schwachen Passwörtern | |
| Zeitfenster 15 Minuten | Bequemer für lange Wartungssitzungen, aber lang genug, dass ein unbeaufsichtigter Browser real gefährlich wird | |
| Du entscheidest | — | |

**Wahl:** Zeitfenster 5 Minuten
**Notizen:** Konfigurierbar.

### Kennzeichnung destruktiver Aktionen

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Deklarativ am Route-Handler | `Destructive: true` in der Route-Registrierung, Middleware liest sie. Eine Stelle, an der Gefahr ablesbar ist; fehlende Markierung ist ein sichtbares Versehen | ✓ |
| Zentrale Allowlist im auth-Paket | Methode+Pfad-Muster an einem Ort, gut testbar, aber räumlich weit vom Handler entfernt | |
| Alles außer GET | Unmöglich zu vergessen, aber die Ausnahmeliste wächst schnell länger als die Regel | |
| Du entscheidest | — | |

**Wahl:** Deklarativ am Route-Handler
**Notizen:** Verbindlich für Phase 6 (Node-Aktionen) und Phase 9 (etcd).

### Session-Lebensdauer

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| 30 Tage absolut, kein Idle-Timeout *(empfohlen)* | Einmal anmelden, einen Monat drin bleiben; eigentliche Verteidigung ist der Sudo-Mode, nicht das Ausloggen | |
| 7 Tage absolut, 12h Idle | Kompromiss; vergessener Tab läuft über Nacht ab, mehr Login-Prompts im Notfall | |
| 24 Stunden | Täglich neu anmelden. Sicher, aber ein Login-Prompt steht im Notfall im Weg | ✓ |
| Du entscheidest | — | |

**Wahl:** 24 Stunden
**Notizen:** Der Betreiber entschied bewusst gegen die Empfehlung — kürzere Sessions sind gewünscht. Konsequenz: Die UI muss Session-Ablauf sauber und ohne Datenverlust behandeln, weil er täglich vorkommt.

### Schutz vor Selbst-Aussperrung

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Nur Verzögerung, nie harte Sperre | Exponentielle Wartezeit pro Quell-IP, ~30s gedeckelt, argon2id ≥250ms. Brute Force wird unmöglich langsam, kein Zustand zum Befreien, keine Hintertür | ✓ |
| Sperre mit CLI-Entsperrung | Klare Semantik, aber Recovery braucht Shell-Zugriff — im Notfall genau das, was fehlt | |
| Sperre, die von selbst abläuft | Einfach, aber ein Angreifer im LAN kann dauerhaft aussperren — DoS gegen das eigene Notfall-Werkzeug | |
| Du entscheidest | — | |

**Wahl:** Nur Verzögerung, nie harte Sperre
**Notizen:** Überstimmt bewusst den „lockout"-Teil von PITFALLS.md:638. Das Fehlen eines Recovery-Pfads ist Absicht — spätere Agenten dürfen keine „Unlock"-Funktion nachrüsten.

---

## UI-Shell & Sprache

### UI-Sprache

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Englisch, keine i18n | Domäne ist ohnehin englisch (MachineConfig, Control Plane, etcd Member, Schematic); durchgereichte API-Fehler ebenso. Planning bleibt deutsch | ✓ |
| Deutsch, keine i18n | Passt zum Planungsmaterial, ergibt aber Mischmasch mit englischen Fachbegriffen und gebrochener Sprache bei API-Fehlern | |
| Englisch mit i18n-Gerüst | Deutsch später nachrüstbar, kostet in jeder Frontend-Phase Disziplin — bei einem Nutzer schwer zu rechtfertigen | |
| Du entscheidest | — | |

**Wahl:** Englisch, keine i18n-Schicht
**Notizen:** Verbindlich für alle späteren Frontend-Phasen.

### UI-Umfang in Phase 1

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Login + App-Shell mit Navigation | Setup, Login, Sidebar mit späteren Bereichen, Header, Toasts, Fehlerseite, Router-Baum; spätere Seiten als „Coming in Phase N" | ✓ |
| Nur Login | Am schnellsten fertig, aber Phase 3 baut die Navigation unter Zeitdruck nebenbei mit | |
| Shell plus Systemstatus-Dashboard | Beweist die Kette an echten Daten, kostet aber Zeit in einer Fundament-Phase | |
| Du entscheidest | — | |

**Wahl:** Login + App-Shell mit Navigation
**Notizen:** Spätere Phasen hängen sich ein, statt umzubauen.

### Theme

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Dark-first, Light per Umschalter | Dunkel als Default, Wahl im localStorage, Default folgt `prefers-color-scheme`. Tailwind 4 bringt `dark:` mit | ✓ |
| Nur Dark | Ein Pfad, kein Umschalter zu testen — aber keine Ausweichmöglichkeit für helle Umgebung oder Doku-Screenshots | |
| System folgen, kein Umschalter | Am wenigsten Code, aber Theme nur über das OS änderbar | |
| Du entscheidest | — | |

**Wahl:** Dark-first, Light per Umschalter
**Notizen:** Dichte Statustabellen mit farbigen Health-Indikatoren lesen sich dunkel besser.

### Komponentenbasis

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| shadcn/ui in den Code kopiert | Radix + Tailwind, per CLI nach `src/components/ui/`. Kein Versions-Vertrag, Zugriff auf jede Zeile, kleines Bundle. Dialog, Command-Palette, dichte Tabellen fertig für spätere Phasen | ✓ |
| Radix-Primitives direkt | Maximale Kontrolle, aber jede Variante selbst bauen — Arbeit, die in Phase 1 nicht bezahlt wird | |
| Mantine/Chakra | Sofort produktiv, bringt aber ein zweites Styling-System neben Tailwind und ein größeres Bundle ins Binary | |
| Du entscheidest | — | |

**Wahl:** shadcn/ui in den Code kopiert
**Notizen:** Bundle-Größe zählt, weil alles ins Binary wandert.

---

## Audit-Log — Form & Sichtbarkeit

### Sichtbarkeit in Phase 1

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Ja — Tabelle mit Filter | Audit-Seite in der Shell: chronologisch, Filter nach Datum und Aktion, Detail-Ansicht. Einziger echter Datenfluss in Phase 1; erstes Tabellen-Muster | ✓ |
| Nein — nur Datei auf Disk | Erfüllt FOUND-06 buchstäblich, aber die UI zeigt nur Platzhalter und die erste echte Tabelle entsteht in der Inventar-Phase | |
| Nur API-Endpunkt | Halber Schritt: Aufwand für Query und Pagination fällt an, sichtbarer Nutzen nicht | |
| Du entscheidest | — | |

**Wahl:** Ja — einfache Tabelle mit Filter
**Notizen:** Beweist die Kette `store → API → UI` an echten Daten statt an Platzhaltern.

### Detailtiefe

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Aktion + Ziel + Parameter, Secrets redigiert | Allowlist ersetzt Passwörter, Schlüssel, PKI-Material. Genug zur Rekonstruktion, ohne dass das Log selbst zum Geheimnisspeicher wird | ✓ |
| Voller Diff vorher/nachher | Maximale Forensik, bläht das Log auf und schreibt Cluster-PKI im Klartext in eine archivierte Datei | |
| Nur Aktion + Ziel + Ergebnis | Kleinste Dateien, kein Redigierungsrisiko — aber der Inhalt eines fehlgeschlagenen Patches fehlt genau dann, wenn man danach fragt | |
| Du entscheidest | — | |

**Wahl:** Aktion + Ziel + Parameter, Secrets redigiert
**Notizen:** Allowlist, nicht Denylist — eine Denylist vergisst das nächste Geheimnis.

### Verifikation der Hash-Kette

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Beim Start automatisch + Banner bei Bruch | Verifiziert aktuelle und zuletzt rotierte Datei; Bruch als dauerhafte UI-Warnung mit betroffener Zeile. Kosten: ein Datei-Durchlauf beim Start | ✓ |
| Button in der Audit-UI | Bewusst ausgelöst und sichtbar, aber niemand drückt ihn, solange nichts verdächtig scheint | |
| CLI-Subcommand | Cron-tauglich, verlangt aber Shell-Zugriff und eine Gewohnheit, die erst entstehen muss | |
| Du entscheidest | — | |

**Wahl:** Beim Start automatisch + Banner bei Bruch
**Notizen:** Begründung wörtlich: eine Kette ohne Prüfung ist Theater.

### Aufbewahrung

| Option | Beschreibung | Gewählt |
|--------|--------------|---------|
| Unbegrenzt, gzip ab dem zweiten Tag | Wenige KB pro Tag bei fünf Nodes. Löschen bräche die Hash-Kette und machte „wann fing das an?" unbeantwortbar | ✓ |
| 90 Tage, dann löschen | Vorhersagbarer Platzbedarf, aber bedeutungslos wenig gespart und Sonderregel für den abgeschnittenen Kettenanfang nötig | |
| Konfigurierbar, Default unbegrenzt | Flexibel für später, kostet jetzt Option, Löschroutine und Kettenregel — Maschinerie für ein Problem, das niemand hat | |
| Du entscheidest | — | |

**Wahl:** Unbegrenzt, komprimiert ab dem zweiten Tag
**Notizen:** Keine Retention-Option in v1 — verworfen, nicht vertagt.

---

## Claude's Discretion

Der Betreiber hat bei keiner der sechzehn Fragen „Du entscheidest" gewählt. Ermessensspielraum besteht nur bei dem, was gar nicht besprochen wurde — aufgelistet in CONTEXT.md `<decisions>` § *Claude's Discretion*: Store-Entity-Zuschnitt, Fehlertaxonomie hinter `problem+json`, Port-Default und Verhalten bei falschen Verzeichnisrechten, Build-/Dev-Workflow, Teststrategie inklusive Crash-Injection, Sidebar-Detailstruktur, Verhalten bei Passwortwechsel, Darstellung des `428`.

## Deferred Ideas

Keine. Die Diskussion blieb durchgehend innerhalb der Phasengrenze; kein Vorschlag hätte eine neue Fähigkeit eingeführt.

Zwei Punkte wurden ausdrücklich **verworfen statt vertagt**, damit sie nicht als offene Frage wiederkehren:

- Retention-Option für das Audit-Log
- Konto-Entsperrung nach Rate-Limit (es gibt keinen Sperrzustand)
