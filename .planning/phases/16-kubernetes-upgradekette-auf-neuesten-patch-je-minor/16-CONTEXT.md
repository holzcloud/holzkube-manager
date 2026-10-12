# Phase 16: Kubernetes-Upgradekette auf neuesten Patch je Minor — Context

Entscheidungen des Betreibers am 2026-10-12 (als Auswahl beantwortet), nach
`16-RESEARCH.md`. Sie gelten für den Planer als **gesperrt**.

## Decisions

- **D-01:** Quelle des neuesten Patches — Live-Lookup `dl.k8s.io/release/stable-1.N.txt`
  (Recherche-Option A, Standard; vom Betreiber nicht abgewählt). Nie
  `latest-1.N.txt` (liefert Release-Candidates). Strikte Validierung der Antwort.
- **D-02:** Lookup-Fehler bei Kette mit Zwischenschritten — der Plan ist
  **blockiert mit einem Satz**, der sagt, dass der neueste Patch nicht ermittelt
  werden konnte. Kein stiller und kein gekennzeichneter Rückfall auf `.0`.
  Einzelschritt-Ziele brauchen keinen Lookup.
- **D-03:** compat-Matrix — wird **nicht** in dieser Phase korrigiert, sondern als
  eigener GSD-Quick-Task **vor** der Ausführung von Phase 16 (Matrix korrigieren,
  Wächter gegen die kompilierte Maschinerie). Phase 16 setzt darauf auf, ändert
  `internal/compat` aber nicht. Der Plan nennt diese Voraussetzung.
- **D-04:** Quell-URL — kein Konfigurationsflag (Recherche-Option A, Standard).
- **D-05:** Durchsetzen — Phase 16 ändert nur, was **angezeigt** wird. Ein
  Mehr-Minor-Ziel wird nicht blockiert; das ist als offener Eintrag in
  `.planning/WINDOWS.md` festzuhalten (Breaking Change, hängt an D-03).

## Bindend aus CLAUDE.md

Öffentliches Repository (keine Kennungen der echten Installation); README im
selben Änderungsschritt, Changelog erst beim Release; jeder Wächter wird gegen den
wieder eingesetzten Fehler rot gesehen; Fragen an den Betreiber als Auswahl.

## Grenzen der Belegbarkeit (im Bericht wörtlich zu sagen)

Nur gegen Simulatoren und `httptest` belegbar. Nicht belegbar: Dauerhaftigkeit des
`dl.k8s.io`-Formats (heute einmal gemessen), Annahme des Zwischenpatches durch
einen echten Cluster, arm64-Lauf.
