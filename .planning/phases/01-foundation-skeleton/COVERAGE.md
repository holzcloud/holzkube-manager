# API Coverage — Phase 01 Foundation Skeleton

No external API integration: Phase 1 baut nur die *eigene* HTTP-Oberfläche und spricht keinen fremden Dienst an — Talos und Image Factory (`factory.talos.dev`) liegen laut ROADMAP in Phase 2.

Im Detail: Phase 1 liefert RFC-9457-Taxonomie sowie Setup-, Login- und Audit-Routen. CONTEXT.md § *Phase Boundary* schließt jede Talos-Interaktion und den Image-Factory-Client ausdrücklich aus dieser Phase aus.

Der deterministische Detektor meldet `detected: true`, sein einziges Signal ist jedoch die Zeichenkette „API-Fehlerantwort" aus einem `must_haves.truths`-Eintrag über die **selbst bereitgestellte** REST-Oberfläche. Es existiert in dieser Phase keine fremde Capability-Oberfläche, über die eine Integrate/Opt-Out-Entscheidung getroffen werden könnte; eine Matrix zu erfinden wäre eine Zeile ohne Gegenstand.

Der laufende Prozess hat in Phase 1 **null** ausgehende Netzwerkabhängigkeiten. Die einzige spätere Egress-Abhängigkeit (`factory.talos.dev`) entsteht in Phase 2 und ist dort matrix-pflichtig.
