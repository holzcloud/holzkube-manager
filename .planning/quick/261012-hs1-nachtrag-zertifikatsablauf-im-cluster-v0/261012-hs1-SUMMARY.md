---
phase: quick-261012-hs1
plan: 01
subsystem: inventory
tags: [certificates, talos, etcd, kubernetes, wall, metrics, retroactive]
status: complete
requires: []
provides:
  - "talos.ClusterClient.Certificates, talos.Certificate, CertificateKind"
  - "internal/certwatch (Ladder, Service, HealthFor), model.CertificateHealth"
  - "ClusterView.node_certificates, wall answer .certificates"
  - "holzkube_node_certificate_seconds{cluster,node,name,kind}"
  - "talossim Options.IssuedCertificateValidity, AuthorityValidity"
affects: [cluster card, CertificateBanner, wall, /metrics, docs/guide.md, docs/api-contract.md, docs/talossim.md, README.md]
key-files:
  created:
    - internal/talos/certs.go
    - internal/talos/certs_test.go
    - internal/certwatch/certwatch.go
    - internal/certwatch/certwatch_test.go
    - internal/model/certificates.go
    - internal/httpapi/handlers/wall_certificates.go
    - internal/httpapi/handlers/wall_certificates_test.go
    - web/src/components/NodeCertificates.tsx
    - web/src/components/NodeCertificates.test.tsx
  modified:
    - internal/talossim/cosi.go
    - internal/talossim/talossim.go
    - internal/talossim/tls.go
    - internal/inventory/inventory.go
    - internal/inventory/view.go
    - internal/metrics/metrics.go
    - internal/metrics/metrics_test.go
    - internal/httpapi/handlers/kubernetes.go
    - cmd/holzkube-managerd/main.go
    - web/src/api.ts
    - web/src/routes/clusters.tsx
    - web/src/routes/wall.tsx
    - web/src/routes/wall.test.tsx
    - web/src/components/CertificateBanner.tsx
    - README.md
    - docs/guide.md
    - docs/api-contract.md
    - docs/talossim.md
    - internal/changelog/changelog.json
commits:
  - 22af6c9 "Certificate expiry inside the cluster ..."
  - dc87868 "Changelog for v0.6.0"
release: v0.6.0 (2026-10-12), CI gate and release verification green
---

# Quick 261012-hs1: Zertifikatsablauf im Cluster — Summary (nachgetragen)

Gebaut am 2026-10-12 **vor** der Entscheidung, mit GSD zu arbeiten, und deshalb
erst nachträglich erfasst. Nichts hier wurde durch GSD-Agenten geplant oder
geprüft.

## Ergebnis

Control-Plane-Nodes werden eine Minute nach Start und danach alle sechs Stunden
über die `secrets.*`-Ressourcen gelesen. Karte, Banner, Wand und `/metrics`
zeigen den Ablauf. Der Gate in CI lief auf `dc87868` grün, die Verifikation des
Release v0.6.0 ebenfalls.

## Geprüft (und wie)

- Fehler wieder eingebaut und rot gesehen: Datum aus den Zertifikaten gelesen
  (nicht Konstante), Leiter-Schwelle für ausgestellte Zertifikate, letzte
  Messung eines ausgefallenen Nodes, Vorzeichen in den Metriken, Node-Leck in
  der Wand, „not read yet", Banner-Schwelle. Zwei Web-Tests waren zuerst
  wirkungslos (Assertion vor dem Rendern) und wurden repariert.
- Lokal (macOS): Go-Tests der berührten Pakete, `go vet`, Biome, 900 jsdom-Tests.
- CI (Release-Lauf auf `dc87868`): vollständiger Gate inklusive Layout und Race.

## Offen

- **Nie gegen einen echten Talos-Cluster gelaufen**, nur gegen `talossim`. Welche
  Ressourcen ein echter Node liefert, zeigt erst der Pi nach dem Update auf
  v0.6.0.
- Keine Fixture in `web/fixtures/demo.json` und kein README-Bild für die neue
  Zeile.
- Auf dem Pi unbestätigt: dass der Update-Timer v0.6.0 installiert hat.
