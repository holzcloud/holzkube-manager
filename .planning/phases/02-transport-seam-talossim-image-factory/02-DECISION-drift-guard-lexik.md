# Entscheidung: was der Browser-Ablehnungs-Waechter ueber `REFUSED_RANGES` behaupten darf

Status: **offen** — vorgelegt, nicht ratifiziert. Empfehlung: Variante B (den Anspruch verengen);
Variante A (Lexer vor dem Anker) liegt **punktgleich** daneben, 7,0 gegen 7,0
Raised by: `02-VERIFICATION.md` (2026-09-04), Runde 6, `gaps[0]` und Human-Verification-Punkt 1
Erarbeitet von: einem autonomen Lauf am 2026-09-05 — vier Entwuerfe, je vier Linsen, jede zitierte
Messung von der pruefenden Linse nachgefahren; **kein Betreiber hat das ratifiziert**
Konsequenz: WINDOWS-Eintrag 70 wird von **keiner** der vier Varianten geschlossen. Er bleibt `open`
und bekommt einen amendierenden Eintrag 72, kein `fixed`.

---

## Das Problem in einem Absatz

`internal/imagefactory/guard_drift_test.go` bindet die Ablehnungsmenge des Servers
(`NotRepresentableReason`, `schematicid.go:321`) an die des Browsers (`REFUSED_RANGES`,
`web/src/routes/images.tsx:105-131`). Die Browser-Seite liest er als **Text**, mit einem regulaeren
Ausdruck: `refusedRangesDecl` (`guard_drift_test.go:108-110`). Sein eigener Fehlertext behauptet
dabei eine allquantifizierte Eigenschaft — *"Renamed, moved or deleted is the same as never having
been there"* (`:265-269`, wortgleich als Doc-Behauptung bei `:254-257`). Diese Eigenschaft ist in
drei aufeinanderfolgenden Verifikationsrunden je einmal falsifiziert worden, jedes Mal um eine Form
enger. Die Frage dieses Dokuments ist nicht, wie das vierte Loch gestopft wird. Sie lautet: **soll
der Waechter mehr messen, oder soll er weniger behaupten** — und wenn mehr messen, mit welchem
Werkzeug.

## Warum das eine Richtungs- und keine Fleissfrage ist

Die Historie ist der Beleg, und sie hat eine Form:

| Runde | Gefunden | Antwort | Ergebnis |
|---|---|---|---|
| 3 | die kanonische Haelfte | geschlossen | — |
| 4 | der Waechter ist gruen, wenn die Tabelle **umbenannt** wird (`RENAMED_BY_VERIFIER`) | Anker an den Bezeichner gebunden | Eintrag 69, bei der Anlage `fixed` |
| 5 | der Waechter ist gruen bei **Praefix-Erweiterung** (`REFUSED_RANGES_LEGACY` -> `ranges=[{0 0}] err=<nil>`, `REFUSED_RANGESX` -> `ranges=[{0 1}] err=<nil>`) | `\s*(?::[^=\n]*)?=` als Wortgrenze | Eintrag 70, `open`, ueberholt 69 |
| 6 | der Waechter ist gruen bei einer Deklaration **ohne lexikalischen Kontext** | offen | dieses Dokument |

Runde 4 und 5 sind eine Familie — der **Name**. Runde 6 hat eine zweite eroeffnet — der
**lexikalische Kontext**. Jede bisherige Antwort war eine Epizykel-Korrektur am Musterstring; die
naechste waere ein Lookbehind, den Gos `regexp` nicht hat. Der Verifier hat den Satz geschrieben, der
diese Entscheidung ueberhaupt erzwingt (`02-VERIFICATION.md:70-75`):

> **Der ehrliche Zusatz, der in diesen Eintrag gehoert:** drei Runden haben je ein engeres Loch in
> derselben allquantifizierten Eigenschaft gefunden, und ein regulaerer Ausdruck ueber
> TypeScript-Quelltext hat einen Rest, der nicht auf null geht.

Das ist eine Aussage ueber die **Technik**, nicht ueber die ausfuehrenden Plaene. Ein Planer, der
"das Loch stopfen" bekommt, waehlt schweigend eine Richtung. Diese Wahl gehoert aufgeschrieben.

## Die Messung, auf der jede Option steht

Woertlich aus `02-VERIFICATION.md:44-48`, Runde 6, selbst gemessen und nicht vom Code-Review
uebernommen:

> Der Anker ist ein regulaerer Ausdruck ueber Quelltext und kennt keinen lexikalischen Kontext.
> Selbst gemessen, nicht vom Code-Review uebernommen: eine `REFUSED_RANGES`-Deklaration, die nur noch
> in einem `/* ... */`-Block lebt, liefert `ranges=[{0 31} {65279 65279}] err=<nil>`; in einem
> Template-Literal `ranges=[{0 31}] err=<nil>`. Der Waechter meldet Zustimmung ueber eine Tabelle,
> die der Browser nicht ausfuehrt.

Der autonome Lauf hat das am echten Baum nachgefahren und dabei **vier Punkte praezisiert**, die die
Optionen unten tragen:

1. **Das Loch ist breiter, als Runde 6 notiert hat.** Weil der Anker mit `(?ms)^\s*` beginnt und
   `\s` Einrueckung wie Zeilenumbruch deckt, erfuellt auch eine **eingerueckte** Deklaration in einem
   `{/* ... */}`-JSX-Kommentar den Anker: gemessen `body=6 whole=6 err=nil`. Genau diese
   Kommentarform ist in `images.tsx` bereits fuenfmal etablierte Konvention (`:597`, `:1155`,
   `:1291`, `:1378`, `:1399`), mit 14 Fortsetzungszeilen, die mit Leerraum statt mit `*` beginnen.
   Ein `/** */`-Block scheitert korrekt, und zwar allein daran, dass seine Fortsetzungszeilen mit `*`
   anfangen.
2. **Der Schadensfall braucht zwei Konjunktionen, nicht drei.** Runde 6 zaehlte drei, weil sie
   `images.tsx:144` fuer eine Nutzung des Bezeichners hielt. `:144` ist Prosa in einem
   JSDoc-Block. Die **einzige** lebende Referenz im ganzen Baum ist `:159`. Die Tabelle ist
   modulprivat; der natuerliche Refactor verschiebt sie in ein gemeinsames Modul und importiert sie
   — das befriedigt `:159` und laesst die alte Tabelle als auskommentierte Leiche stehen. Gemessen
   (`import { REFUSED_RANGES } from '../lib/refused-ranges'` plus die echte Tabelle in einem
   `/* */`-Block): `body=6 whole=6 unreadable=0` -> **GUARD GREEN** ueber sechs Phantom-Bereiche. Der
   Zaehlvergleich schuetzt nicht, weil er die Leiche mitzaehlt.
3. **Der Spiegel-Defekt ist heute latent, nicht live.** `images.tsx` traegt genau sechs
   eintragsfoermige Literale, alle sechs in der Deklaration. Eine geleerte `= []` gibt heute die
   richtige Meldung; erst ein zweites eintragsfoermiges Literal irgendwo in der Datei kippt sie in
   die falsche. Siehe eigener Abschnitt weiter unten.
4. **Die Runden 4-6 sind gegen den echten Baum wirklich geschlossen.** `REFUSED_RANGES_LEGACY` ->
   `DECL=nil`; Deklaration hinter `//` -> `DECL=nil`; echte Tabelle plus zweite Tabelle -> Zaehl-
   Fehlschlag. Es geht, wie in den Runden 3, 4 und 5, um das **kuenftige** Verhalten des Waechters —
   die beiden Mengen stimmen heute Klausel fuer Klausel ueberein.

---

## Variante A — Ehrlicher Quelltext: der Lexer vor dem Anker

Der Anker bleibt Byte fuer Byte, was ihm gereicht wird, aendert sich: ein handgeschriebener,
byteweiser TSX-Lexer (`stripNonCode`, ~180 Zeilen nach Entwurf, realistisch 550-750 in der
Kommentarquote dieses Pakets) leert jeden Kommentar, jede Zeichenkette und jedes Template-Literal
laengentreu zu Leerzeichen. Er wird an fuenf Invarianten gehalten (Laenge, Zeilenzahl,
nur-Leerzeichen, Idempotenz, Offset-Abbildung fuer die Fehlerzitate) plus einer Klammerbilanz-Sweep
ueber alle 49 `.ts`/`.tsx`-Dateien unter `web/src`.

**Was es schliesst.** Die gesamte lexikalische Familie auf einen Schlag, nicht Mitglied fuer
Mitglied. Nachgefahren: alle drei Runde-6-Formen — blosser `/* */`-Block, eingerruecktes
`{/* */}`, Template-Literal, jeweils mit dem echten Import daneben — gehen von GREEN auf Exit (a).
Die echte Datei liest weiterhin genau sechs Bereiche. Der Spiegel-Defekt entfaellt ersatzlos, weil
`whole` seine zweite Aufgabe verliert. Als Zugabe faellt ein Loch, das niemand benannt hatte: eine
geschattete innere Deklaration wird von der neuen Duplikatspruefung erwischt.

**Was es kostet.** Null Abhaengigkeiten — kein Go-Modul, kein Node, kein Netz, kein Codegen, kein
`go:generate`, keine CI-Aenderung; `.golangci.yml` hat kein Komplexitaetsgate, an dem eine dichte
Zustandsmaschine haengenbliebe. Dafuer ein zweites `guard_drift_test.go` an Volumen, und eine
**dauerhafte Kopplung** der Go-Suite an das gesamte Frontend: der Bilanz-Sweep macht jede
`.ts`-Datei unter `web/src` zur moeglichen Ursache eines roten Go-Waechters. Zwei bestehende
Testzeilen kippen ihr Verhalten, und `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` bricht,
solange die Meldungen nicht auf die Quellsicht umgestellt sind — die gestrippte Form eines Eintrags
lautet gemessen `{ from: 0, to: 31, class:` + 21 Leerzeichen + `}`, waehrend `:551` das Literal mit
`'control character'` verlangt.

**Was es nicht schliesst — und das ist der Kern.** Die allquantifizierte Behauptung wird
**substituiert, nicht eliminiert**: aus "der Anker sieht nur Code" wird "`stripNonCode` erkennt Code
in allem TSX". Der Entwurf misst selbst vier Eingaben, an denen das falsch ist. Die pruefende Linse
hat die gefaehrliche Richtung dazu **ausgefuehrt**: eine Tabelle als reiner JSX-Textknoten in einem
`<pre>`-Block, neben dem echten Import, ohne Schraegstrich, Backtick oder Anfuehrungspaar, ergibt
`decl=1 entries=6 whole=6 balanced=true` — **GUARD GREEN**. Sie passiert alle fuenf Invarianten *und*
die Klammerbilanz. Der Entwurf nennt JSX-Text nur in der harmlosen Fehl-Leerungs-Richtung (falsches
Rot); die Falsch-Gruen-Richtung steht in vier Woertern. Ausserdem unberuehrt: `images.tsx:159` als
einzige lebende Bindung zwischen Tabelle und Formular, und `stringArrayLiteral`
(`guard_drift_test.go:667`) mit exakt dem Praefix-Loch aus Runde 5.

**Panel:** Wahrhaftigkeit 8, Robustheit 7, Kosten 6, Passung 7 — **Schnitt 7,0**.

## Variante B — Anspruch verengen: der Waechter sagt Text, nicht Programm

Der Anker bleibt, der Anspruch schrumpft auf das Gemessene. Sechs Aenderungen: die Behauptung im
Doc-Kommentar wird durch eine Aussage ueber den **Text** plus eine geschriebene Ausschlussliste
ersetzt; Exit (a) bekommt eine unterscheidende Phrase und nennt die Blindheit; der Spiegel-Defekt
wird per Rumpfform beseitigt; `wantErr` wird `[]string` und pinnt unterscheidende Saetze statt der
Zeichenkette `REFUSED_RANGES`, die sechs von sieben Meldungen tragen; eine vierte Tabelle
(`TestBrowserRefusalGuardIsBlindToNonCodeContext`) nagelt die Blindheit mit **umgekehrter Abnahme**
fest — sie verlangt `err == nil` und geht rot an dem Tag, an dem die Blindheit endet; dazu ein
Ledger-Eintrag fuer den Rest.

**Was es schliesst.** Die *Behauptung* geht auf null Ueberhang. Was danach in der Datei steht, ist
wahr: alle drei Runde-6-Formen sind nachgefahren gruen (`body=2 whole=2 err=nil`), und genau das
sagt der Waechter kuenftig von sich. Der Spiegel-Defekt verschwindet. Die drei schwachen
`wantErr`-Zeilen werden aus Schein-Zusagen echte. Die **Aritaet** des Rests aendert sich: eine
allquantifizierte Behauptung hat unendlich viele Gegenbeispiele und jedes ist eine Falsifikation;
eine geschriebene Liste hat endlich viele Eintraege und jede neue Form ist eine Ergaenzung.

**Was es kostet.** Keine Abhaengigkeit, kein neuer Import — `strings`, `regexp`, `fmt`, `testing`
stehen schon im Block. `GOPROXY=off go test ./internal/imagefactory/` bleibt offline gruen (heute
gemessen 0,54 s). Rund 150 Zeilen Prosa und Test, plus die Zeremonie: dieses Dokument, eine
Ledger-Transaktion und — als einziges Geraet ohne Test — drei Kommentarzeilen ueber
`images.tsx:105`, die alles darunter um drei Zeilen verschieben und damit rund ein Dutzend
`images.tsx:NNN`-Verweise im Planungsbestand ungueltig machen.

**Was es nicht schliesst.** Den **Defekt**. Die Leiche bleibt lesbar, der Waechter bleibt gruen
darueber, und der Zeitpunkt, an dem das schadet, ist unveraendert. Zwei weitere Befunde der
pruefenden Linsen: (i) `TestBrowserRefusalGuardIsBlindToNonCodeContext` ruft die reine Funktion
direkt, waehrend `browserRefusalRanges` (`:243`) den Quelltext liefert — ein kuenftiges
`stripNonCode` **am Aufrufer** statt in der Funktion laesst die Blindheitstabelle gruen, und der
unbedingte Satz "goes red the day the blindness ends" ist damit selbst allquantifiziert und
verletzbar; (ii) die Blindheit ist als *non-code* skopiert, waehrend der gefaehrlichste Rest in
**lebendem Code** sitzt (siehe "Was keine der vier Varianten schliesst").

**Panel:** Wahrhaftigkeit 8, Robustheit 6, Kosten 6, Passung 8 — **Schnitt 7,0**.

## Variante C — Eine Quelle, ein generierter Spiegel

Die Regel wird nur noch **einmal** ausgesprochen. `schematicid.go` bekommt einen Klassifizierer,
ein Sweep ueber alle 1.114.112 Codepoints verdichtet ihn zu Bereichen, und ein Testfile emittiert
daraus ein committetes `web/src/lib/refused-ranges.generated.ts`. Der Waechter schrumpft auf
`os.ReadFile` plus Byte-Vergleich; rund 390 der 711 Zeilen entfallen samt allen sieben
Fehlerausgaengen und dem Spiegel-Defekt.

**Was es schliesst.** Die lexikalische Frage ist danach **nicht mehr ausdrueckbar**: es gibt keinen
zweiten Text, den ein Ausdruck lesen muesste. Umbenennung ist ein `tsc`-Fehler, eine Leiche ist Text,
den niemand liest. Der Sweep reproduziert die heutigen sechs Eintraege wert- und klassengleich, die
Umstellung ist also verhaltensgleich.

**Was es kostet.** Am meisten von allen vieren, und an der teuersten Stelle. Zwoelf Dateien. Die
**erste generierte, committete Datei** dieses Repos — `git grep "DO NOT EDIT|Code generated|
go:generate"` ist heute leer, und `Taskfile.yml` lehnt `go:generate` namentlich ab. Ein
ratifizierungspflichtiger Blockier-Checkpoint vor dem ersten Commit. Und ein Umbau von
`NotRepresentableReason` — der Funktion, auf der FACT-06 ruht, mit 116 Zeilen gemessener Belege in
ihren Klauselkommentaren und einem Doc-Kommentar, der *"Run the instrument before changing this
function"* verlangt, wo das Instrument (`TestLiveCanonical`) opt-in ist und in keiner Pipeline
laeuft (Eintrag 64).

**Was es nicht schliesst.** Die Grenze wird verschoben, nicht aufgehoben: aus "sieht der Ausdruck
den Kontext" wird "ruft das Formular das Praedikat auf". Gemessen von der pruefenden Linse: das
Formular reimplementiert das Praedikat lokal (`FORM_RANGES` mit zwei statt sechs Bereichen), alles
kompiliert, `noUnusedLocals` greift nicht, **alle Suiten gruen**, waehrend das Formular U+2028,
U+FEFF und alles ueber U+FFFD wieder annimmt. Dazu: eine Go-Klausel, die eine Grenze verschiebt
(`0x9F` -> `0xA0`), ohne Bereichs- oder Lueckenzahl zu aendern, entgeht der 6+7-Stichprobe des
Verdrahtungstests — und `schematicid.go:311` sagt selbst, diese Menge sei *"a floor, not a ceiling"*.

**Panel:** Wahrhaftigkeit 8, Robustheit 6, Kosten 5, Passung 6 — **Schnitt 6,3**.

## Variante D — Bindung statt Fundstelle

Ein Node-Skript unter `web/tools/` laesst `@babel/parser` 7.29.8 (liegt bereits transitiv in
`web/node_modules`) `images.tsx` parsen und gibt das an den modulweiten `const REFUSED_RANGES`
gebundene Array-Literal als JSON zurueck. Go ruft es ueber stdin, faellt **hart** statt zu skippen,
wenn `node` oder `node_modules` fehlen. Alle drei Regexe verschwinden.

**Was es schliesst.** Die lexikalische Familie **von Bauart**, nicht durch Aufzaehlung: ein
Bindungstabellen-Eintrag kann keinen Kommentartext enthalten. Nachgefahren gehen ausser den drei
bekannten Formen auch vier nie aufgezaehlte auf `bound=false` — Objekt-Property, Re-Export, in einer
Funktion verschachtelt, eingerruecktes JSX-Kommentar. Der Schadensfall aus Runde 6 wird nicht bloss
abgelehnt, sondern **diagnostiziert**: `imported from '../lib/refused-ranges'`. Die
Trunkierungsklasse und mit ihr der Spiegel-Defekt entfallen ersatzlos. Laufzeit gemessen: 0,11 s
kalt, ~2,2 s fuer 20 Tabellenzeilen.

**Was es kostet.** Der erste Go-Test dieses Repos, der einen Subprozess einer **fremden** Toolchain
startet — die drei vorhandenen Shell-outs rufen nur das Go-Werkzeug. Der geschriebene Kopfsatz
*"a Go test reads a TypeScript literal with the standard library"* (`budget_drift_test.go:24`) wird
damit falsch und muss mitgeaendert werden. `GOPROXY=off go test ./internal/imagefactory/` ist heute
in 0,54 s **ohne node** gruen und danach rot; die Rechtfertigung ("`go test ./...` ist ohne Bundle
ohnehin rot") ist erschlossen, nicht gemessen, und sie liegt im falschen Paket. Der Leser selbst ist
tragend und von nichts gedeckt: eine Lockerung darin faerbt alle 20 Falsifikationszeilen gruen.

**Was es nicht schliesst.** Dieselbe Ableitungsluecke wie A, B und C — gemessen:
`const HARD = REFUSED_RANGES.filter((r) => r.class !== 'above U+FFFD')` ergibt
`entries:6 referenced:1` -> gruen. Zusaetzlich streicht D die Verschmutzungspruefung ersatzlos: eine
zweite, vom Formular tatsaechlich benutzte Tabelle plus **ein** kosmetischer Lesezugriff
(`{REFUSED_RANGES.length}`) ist unter D gruen, waehrend der heutige Waechter darueber rot ist. Und
D's eigene Spezifikation ("Bindungssuche nur in `ast.program.body`") meldet fuer ein harmloses
`export const REFUSED_RANGES` faelschlich `not-declared` — fail closed, aber falsche Ursache, ohne
Tabellenzeile.

**Panel:** Wahrhaftigkeit 8, Robustheit 6, Kosten 5, Passung 6 — **Schnitt 6,3**.

---

## Das Panel-Ergebnis, und wie knapp es ist

| Variante | Wahrhaftigkeit | Robustheit | Kosten | Passung | Schnitt |
|---|---|---|---|---|---|
| A — Lexer vor dem Anker | 8 | 7 | 6 | 7 | **7,0** |
| B — Anspruch verengen | 8 | 6 | 6 | 8 | **7,0** |
| C — generierter Spiegel | 8 | 6 | 5 | 6 | 6,3 |
| D — Parser-Bindung | 8 | 6 | 5 | 6 | 6,3 |

**A und B sind exakt punktgleich, und C und D ebenfalls.** Das steht hier so deutlich, weil ein
knappes Ergebnis als klares darzustellen genau der Defekt waere, den diese Phase seit fuenf Runden
korrigiert. A gewinnt die Robustheit um einen Punkt, B die Passung um einen. Wer Kosten und
Robustheit hoeher haengt als Passung, kommt bei A heraus, und das ist keine Fehlbewertung, sondern
eine andere Gewichtung derselben Zahlen.

Bemerkenswert und fuer die Empfehlung tragend: **alle vier Varianten bekommen Wahrhaftigkeit 8.**
Keine der vier hat einen unbelegten Kern; jede benennt ihren eigenen Rest. Die Entscheidung faellt
also nicht zwischen ehrlich und unehrlich, sondern zwischen vier ehrlichen Antworten auf die Frage,
wie viel gemessen werden soll.

## Empfehlung

**Variante B jetzt. Variante D ist das strukturell richtige Ende und gehoert in die Phase, die die
Web-Naht ohnehin wieder oeffnet.**

Drei Gruende, in absteigender Staerke:

**1. Zwei unabhaengig ausgefuehrte Messungen sagen, dass Weg (a) das Muster nicht beendet.** Die
Robustheits-Linse hat A's eigenen Lexer gebaut und mit einer Tabelle als JSX-Textknoten in einem
`<pre>`-Block gruen bekommen — alle fuenf Invarianten und die Klammerbilanz passiert. Und die
B-Linsen haben den in `02-REVIEW.md:164-180` **entworfenen** `stripNonCode` woertlich nachgebaut:
`const doc = ` + Backtick + `${` + Backtick + Zeilenumbruch + Tabelle + Zeilenumbruch + Backtick +
`}` + Backtick bleibt danach gruen mit `body=2 whole=2`, weil die Backticks (1,2) und (3,4) paaren
und die Tabelle in der Luecke liegt; dazu zwei **neue** Falsch-Rot-Faelle auf lebenden, korrekten
Tabellen. Weg (a) verschiebt den Rest also und behaelt die allquantifizierte Behauptung — genau die
Kombination, die dreimal gescheitert ist. Runde 8 haette bei A ein fertiges Ziel.

**2. B ist die Operation, die diese Phase seit Runde 3 von jedem anderen Artefakt verlangt.** Der
Verifier hat das ausgeschrieben (`02-VERIFICATION.md:520-522`): *"Das ist kein Nachgeben, sondern
genau die Operation, die diese Phase seit Runde 3 von jedem anderen Artefakt verlangt: nicht mehr
behaupten, als gemessen wurde."* Plan 02-27 hat seine eigene Luecke aus demselben Grund nicht
geschlossen erklaert. B wendet die Regel zum ersten Mal auf den Waechter selbst an.

**3. Der Phasenstand und der einzige ratifizierte Praezedenzfall zeigen in dieselbe Richtung.** Plan
28 von 28, `status: executing`, Verifikationsrunde 6, 55 offene Fenster, eine unerledigte
Buchhaltungsregression daneben. `02-DECISION-probe-budget.md:106-111` stand vor exakt dieser Wahl,
nannte die grosse Loesung *"the structurally correct end state"* und verschob sie, *"because it
introduces a probe state and a route at the moment the phase is trying to close"*. A ist die
mittlere, C und D sind die grossen Loesungen dieser Frage.

Und die Gegenprobe, weil eine Empfehlung ohne sie nichts wert ist: **B waere falsch, wenn Weg (a)
den Rest wirklich auf null braechte.** Dann waere die Verengung tatsaechlich Nachgeben. Er bringt ihn
nicht auf null, und das ist gemessen und nicht erschlossen.

### Was B in Runde 7 anders bekommen muss, als der Entwurf es schreibt

Vier Korrekturen, alle aus den Pruefungen, alle notwendig:

- **Die Blindheit wird nach MECHANISMUS skopiert, nicht als "non-code".** Der Testname, die
  Ausschlussliste und der Ledger-Eintrag muessen sagen: *der Anker bindet an ein Literal, nicht an
  einen Wert, und an einen Bezeichner, nicht an Code.* Sonst laesst sich der Eintrag buchstabengetreu
  schliessen, waehrend der Ableitungspfad offen und ungeledgert bleibt.
- **Die Blindheitstabelle laeuft ueber denselben Pfad wie der Live-Waechter**, nicht ueber die reine
  Funktion allein — sonst ist ihr Anti-Rot-Versprechen mit einem `stripNonCode` am Aufrufer zu
  umgehen.
- **Der `honestClaim` bekommt einen fuenften Ausschluss:** der Sweep ueberspringt D800-DFFF
  (`:154-156`) und prueft die Surrogat-Menge nur an ihren beiden Endpunkten (`:144-149`); ihr
  Server-Zwilling ist `rawBodyRefusal` und wird nie aufgerufen. Fuer 2048 Codepoints wird nichts
  verglichen. Das gehoert neben die Behauptung, nicht in ein anderes Dokument.
- **`overrides:` wird nicht benutzt.** Beide Verifikationen tragen `overrides_applied: 0`, die
  einzige Schema-Instanz steht ungehoben in einem Fence bei `01-VERIFICATION.md:476-486`, und das
  Ledger-Verb fuer bewusste Annahme heisst `waive` (`waived_count: 0`). Die Ratifikation landet im
  Statusblock dieses Dokuments und im Text von Eintrag 72. Das erfuellt `02-VERIFICATION.md:436`
  ("in `.planning/WINDOWS.md` **bzw.** in den `overrides:`") auf der ersten Haelfte der Disjunktion.

## Die staerksten Einwaende gegen diese Empfehlung

Sie stehen hier, weil eine Empfehlung, die ihre Gegenrede nicht traegt, in dieser Phase nichts wert
ist.

**1. Der Code-Review ordnet anders, und er ordnet begruendet.** `02-REVIEW.md:160-162` schreibt:
*"Kommentare und Zeichenketten vor dem Anker aus der Quelle entfernen, statt den Anker weiter zu
verfeinern — der Anker kann diese Unterscheidung strukturell nicht treffen."* Die Verengung erscheint
dort erst bei `:182-186` und ausdruecklich als *"Falls das als zu teuer gilt"*. Nur der Verifier hat
(b) zur Gleichrangigkeit erhoben. Diese Empfehlung verlangt vom Betreiber, die Ordnung des Reviews zu
ueberstimmen.

**2. B entfernt die Behauptung, nicht den Defekt.** Der Leichen-Fall bleibt erreichbar, und unter A
waeren zwei seiner drei Formen es nicht. Wer haelt, dass ein Waechter zu bewachen und nicht sich zu
beschreiben hat, liest B als die Phase, die sich aus einer Reparatur herausredet.

**3. Die naechsten Funde wandern in die Prosa.** Unter B findet Runde 8 Saetze statt Muster: "die
Ausschlussliste nennt `${...}` nicht", "die dritte Blindheitszeile ueberzeichnet den Refactor". Das
ist dieselbe **Form** von Fund eine Ebene hoeher, und Prosa hat keinen Compiler. B's einziger
Mechanismus dagegen ist die Blindheitstabelle, und die pinnt drei Quellen, nicht die Klasse.

**4. Der Abstand ist null.** A hat die hoehere Robustheit, und die Robustheits-Linse ist die, die in
dieser Phase dreimal recht behalten hat. Eine Gewichtung, die Robustheit ueber Passung stellt,
liefert A — und diese Empfehlung waere dann falsch, nicht bloss anders.

## Was die Empfehlung fuer WINDOWS-Eintrag 70 bedeutet

**Eintrag 70 bleibt `open`, und er wird von dieser Runde nicht geschlossen.** Er wird auch nicht
umgeschrieben: sein eigener Text sagt, dass es kein amend-Verb gibt. Die Form, die Eintrag 66 gegen
58 etabliert hat, ist die richtige: ein neuer Eintrag, der amendiert und den alten offen stehen
laesst.

Warum nicht `fixed`, obwohl 70s geschriebene Schliessbedingung von Runde 6 **woertlich ausgefuehrt
und in beiden Richtungen erfuellt** wurde (`02-VERIFICATION.md:27`, `:265-266`)? Weil 70 ueberhaupt
nur existiert, weil 69 bei der Anlage `fixed` gesetzt wurde, waehrend die allquantifizierte
Eigenschaft weiterlebte, und weil 70 das *"genau der Fehler von 69"* nennt und ausdruecklich als
mechanisch und nicht bloss stilistisch bezeichnet. Ein `fixed` waere formal dieselbe Bewegung. Dass
die geschriebene Bedingung und der tatsaechliche Offen-Grund auseinandergelaufen sind, ist ein echter
Mangel — er wird durch den Text von 72 behoben, nicht durch ein `fixed` an 70.

**Neuer Eintrag 72**, `kind: unmet-truth`, `status: open`, `file:
internal/imagefactory/guard_drift_test.go`, **Zeilenspalte leer** — er zeigt auf den Symbolnamen
`refusedRangesDecl`, wie `02-REVIEW.md:510-521` (IN-05) es als die haltbare Variante verlangt; 70s
`line = 83` ist seit 02-27 stale und wird dadurch mit erledigt. Er oeffnet mit *"AMENDS ENTRY 70,
WHICH STAYS OPEN (no amend verb exists)"* und traegt die Abschnitte in 70s Kapitaelchen-Form:
was ueberholt ist, was an 70 weiterhin gilt, was Runde 7 getan hat, warum er offen bleibt und was ihn
schliesst, die Nachbarschaft (56 bleibt offen und wird nicht ueberholt; 58 und 66 tragen G-02-9
unberuehrt; `stringArrayLiteral` und `budget_drift_test.go:89-90` sind **nicht** gedeckt).

`open_count` 55 -> 56. Die Schiffsbremse steht bei 55 ohnehin; zwei Zahlen weiter oder naeher
aendert an `workflow.windows_enforce` nichts. Genau deshalb ist an dieser Buchung nichts zu
gewinnen, und genau deshalb ist sie glaubwuerdig.

### Die Schliessbedingung von Eintrag 72, pruefbar und endlich

Eine Verifikationsrunde schliesst 72, wenn sie gegen den dann geltenden Waechter **alle sechs**
Punkte misst und protokolliert:

1. `REFUSED_RANGES` praefixiert umbenannt (`_LEGACY`) -> **rot**. (Regressionssperre Runde 5.)
2. Deklaration nur hinter `//` -> **rot**. (Regressionssperre.)
3. Ein Eintrag mit einer Grenze oberhalb `utf8.MaxRune` -> **rot**. (Regressionssperre Runde 6, zweite
   Haelfte von 70s alter Bedingung.)
4. Eine wirklich leere `= []` neben einer zweiten eintragsfoermigen Tabelle -> die **Leer**-Meldung,
   nicht *"Look for that bracket"*. (Spiegel-Defekt.)
5. Die geschriebene Ausschlussliste wird gegen jede darin genannte Form **einzeln gemessen** und die
   gemessene Ausgabe im Protokoll zitiert — mindestens: blosser `/* */`-Block, eingerruecktes
   `{/* */}`, Template-Literal (je mit dem Import daneben, sonst ist die Fixture kein Schadensfall,
   sondern ein TS-Fehler), sowie die Ableitungsformen `].filter(...)` und `...SPREAD`. Jede Form, die
   die Liste nennt, muss die Ausgabe liefern, die die Liste behauptet.
6. Die Runde **versucht ausdruecklich**, eine **siebte** Form zu bauen, die die Liste nicht nennt
   (JSX-Text, `String.raw`, Regex-Literal, `${...}`-Interpolation, `#private`-Name), und
   protokolliert das Ergebnis. Findet sie eine, die der Waechter liest und die Liste nicht nennt,
   **bleibt 72 offen** und die Liste wird um sie erweitert.

Punkt 6 ist die eigentliche Huerde und der Grund, warum diese Bedingung diese Empfehlung widerlegen
kann. Ein Eintrag, dessen Bedingung nur die schon bekannten Formen abfragt, waere ein Eintrag, der
sich selbst schliesst.

## Der Spiegel-Defekt der Runde 6 — unabhaengig von der Richtungsfrage

`whole` wird bei `guard_drift_test.go:300` ueber die **ganze Quelle** gezaehlt, und dieselbe Zahl
beantwortet danach zwei verschiedene Fragen: *"war der Rumpf abgeschnitten?"* (`:307`) und *"steht
ein Literal ausserhalb der Deklaration?"* (`:329`). Folge, von Runde 6 gemessen: eine **wirklich
leere** `const REFUSED_RANGES ... = []` bekommt in einer Datei mit irgendeinem zweiten
eintragsfoermigen Literal die Meldung *"the declaration body was cut short before its first entry ...
Look for that bracket"* — fuer eine Klammer, die es nicht gibt. Vor dieser Runde war genau dieser
Fall richtig diagnostiziert. Eine falsche Ursache wurde gegen eine andere getauscht.

Verschaerfend, und im Register dieser Phase das Eigentliche: die Meldung behauptet einen Satz, den
der Code nie feststellt — *"The declaration is NOT empty"* wird aus einer Dateizaehlung erschlossen,
nicht aus einer Eigenschaft der Deklaration.

**Der Fix ist in jeder der vier Varianten derselbe und steht nicht zur Debatte** (`02-REVIEW.md:235-265`,
identisch als eigener `missing`-Punkt bei `02-VERIFICATION.md:88`): die unterscheidende Groesse ist
die **Form des Rumpfes**, nicht eine Zaehlung ueber die Datei.

```go
if len(matches) == 0 {
    if strings.TrimSpace(body) == "" { /* ...declared but empty... */ }
    /* ...cut short..., und die Meldung zitiert body mit %q... */
}
```

Dazu eine Tabellenzeile *"eine wirklich leere Deklaration neben einer zweiten Tabelle"*, die heute
die falsche Meldung bekommt, und die Verkuerzung des `wantErr` der bestehenden `truncated`-Zeile auf
`"cut short before its first entry"`, damit sie gruen bleibt.

**Was auch danach bleibt, und was gesagt werden muss:** `strings.TrimSpace(body) == ""` haelt einen
Rumpf aus `//\n` oder `/* */` fuer nicht leer und nimmt den Abschneide-Zweig. Kleiner als der Defekt,
den es ersetzt — die Meldung zeigt jetzt ihren Beleg statt ihn zu behaupten —, aber es ist dieselbe
Gattung, innerhalb der Aenderung, die diese Gattung beseitigen soll. Das gehoert in 72, nicht in eine
Fussnote.

**Heute ist der Defekt latent, nicht live:** `images.tsx` traegt sechs eintragsfoermige Literale, alle
sechs in der Deklaration, also gibt eine geleerte Tabelle heute die richtige Meldung. Latent ist kein
Grund zu warten — es ist der Grund, warum ihn niemand bemerkt haette.

## Was keine der vier Varianten schliesst

Diese Punkte sind kein Argument fuer oder gegen eine Option, sondern der Rest, der in jedem Fall
bleibt und in Eintrag 72 gehoert:

- **Die Ableitung.** Der Anker (und ebenso Babels Bindung, und ebenso ein Byte-Vergleich gegen ein
  generiertes Modul) bindet an ein **Literal**, nicht an einen **Wert**. Gemessen, in lebendem,
  kompilierendem, bei `:159` referenziertem Code: `].filter((x) => x.class !== 'byte order mark')`
  -> sechs Bereiche gelesen, `err=nil`, waehrend das Formular U+FEFF wieder annimmt und der Server es
  weiter ablehnt. Ebenso `...PLATFORM_RANGES` und `].slice(...)`. Das ist **gruen auf echter Drift**,
  dieselbe Schwere wie G-02-11 selbst, und es ist der gefaehrlichste bekannte Rest.
- **Die eine tragende Verbindung.** `images.tsx:159` ist die einzige lebende Referenz auf
  `REFUSED_RANGES` im ganzen Baum. Faellt der Aufruf von `hasControlCharacter` aus dem
  Validierungspfad, ist die Tabelle eine wohlgeformte, lebende, uebereinstimmende Leiche, und jede
  der vier Varianten ist gruen darueber. G-02-17 war genau dieser Fall an einem einzelnen Feld, und
  gefunden hat ihn ein Mensch.
- **Die Geschwister.** `stringArrayLiteral` (`guard_drift_test.go:667`, Muster
  `NAME[^=]*=\s*\[([^\]]*)\]`) traegt Runde 5s Praefix-Loch unveraendert weiter;
  `budget_drift_test.go:89-90` und `warnings_test.go:281-336` ebenso. Keine Variante fasst sie an. Sie
  stillschweigend stehen zu lassen waere dieselbe Operation, die 69 falsch gemacht hat — sie gehoeren
  in einen eigenen offenen Eintrag.
- **Die Surrogat-Haelfte.** Der Sweep ueberspringt D800-DFFF, die Endpunkte werden einzeln behauptet,
  das Innere nie geprueft, und der Server-Zwilling `rawBodyRefusal` (`schematics.go:1080`) wird nicht
  aufgerufen. Unveraendert seit 02-20.
- **Eintrag 56** — dass die SERVER-Menge nicht erschoepfend gegen `factory.talos.dev` gemessen ist und
  02-14s Extrapolation erbt — wird von keiner Variante um ein Byte besser und bleibt offen.

## Was in jedem Fall gilt

Unabhaengig von der gewaehlten Richtung, und deshalb ohne Ratifikation planbar:

- Der Spiegel-Defekt wird ueber die Rumpfform beseitigt, nicht getauscht (eigener Abschnitt oben).
- Die drei Zeilen mit `wantErr: "REFUSED_RANGES"` (`:470`, `:475`, `:485`) werden auf je eine
  **unterscheidende** Teilzeichenkette gehoben. Sechs von sieben Reader-Meldungen tragen den
  Bezeichner; diese drei Zeilen pinnen heute "irgendein Fehler", nicht "der Anker hat nicht
  getroffen" (`02-REVIEW.md` WR-04).
- `from > to` in der Bound-Meldung bekommt seine **eigene** Ursache (ein aufwaerts laufender Sweep
  betritt eine invertierte Range nie) statt der von `to > utf8.MaxRune` (`02-REVIEW.md` WR-03).
- Eintrag 70s stale Zeiger auf Zeile 83 wird von Eintrag 72 mit erledigt, indem 72 auf einen
  **Symbolnamen** zeigt und die Zeilenspalte leer laesst (IN-05).
- Kuenftige Ledger-Eintraege ueber Code zitieren Symbole, nicht Zeilennummern. Das ist die dauerhafte
  Antwort auf IN-05 und zugleich die Antwort auf die Nebenwirkung von B's Hinweiszeile ueber
  `images.tsx:105`: sie verschiebt alles darunter um drei Zeilen und macht rund ein Dutzend
  `images.tsx:NNN`-Verweise im Planungsbestand ungueltig. Wird die Zeile geschrieben, gehoert die
  Korrektur dieser Verweise in dieselbe Aenderung; sie ist das einzige Geraet dieses Entwurfs, das
  den Menschen erreicht, der die Leiche erzeugt, und deshalb ihren Preis wert.

## Ratifikation

**Ausdruecklich offen.** Diese Empfehlung ist von einem autonomen Lauf am 2026-09-05 erarbeitet
worden: vier Entwuerfe, je vier Linsen, und jede zitierte Messung von der pruefenden Linse an einem
eigenen Harness nachgefahren statt uebernommen. **Kein Betreiber hat sie ratifiziert.**

Der Kontrast innerhalb dieser Phase ist der Massstab: `02-DECISION-probe-budget.md` wurde vom
Betreiber in Sitzung ratifiziert, und Runde 4 war bis dahin blockiert;
`02-DECISION-schematic-identity.md` wurde vom Ausfuehrenden selbst aufgeloest und traegt das im Text
(*"self-resolved, not ratified"*). Dieses Dokument liegt naeher am zweiten Fall, aber mit einem
Unterschied, der genannt gehoert: der Verifier hat diese Frage ausdruecklich als
Human-Verification-Punkt 1 dem Betreiber zugewiesen (`02-VERIFICATION.md:430-440`, `:522-523`:
*"Welcher Weg genommen wird, gehoert dem Betreiber"*). Sie autonom zu **beantworten** war der
Auftrag; sie autonom zu **entscheiden** waere es nicht.

**Solange nichts anderes gesagt wird, plant Runde 7 auf dieser Grundlage** — Variante B mit den vier
Korrekturen oben, dem unbedingten Teil, Eintrag 70 offen und Eintrag 72 neu. Der Wechsel auf Variante
A kostet den Betreiber **einen Satz**: die Kosten sind bezahlt, solange nichts implementiert ist, der
unbedingte Teil ist von der Richtungsfrage unabhaengig, und beide Varianten stehen oben in gleicher
Ausfuehrlichkeit. C und D sind fuer die Phase, die die Web-Naht ohnehin wieder oeffnet, in voller
Behandlung mitgeschrieben.

Zur Sprache, weil beide Vorgaengerdokumente englische Koerper haben: dieses ist deutsch, wie die
uebrigen Planungsartefakte dieser Phase und wie der juengste Text in einem Entscheidungsdokument
(`02-DECISION-schematic-identity.md`, Nachtrag vom 2026-09-04). Bezeichner, Pfade, Code und die
zitierten Saetze aus dem Baum bleiben englisch.

Wird ratifiziert, gehoert die Ratifikation in den Statusblock oben und in den Text von Eintrag 72 —
und ein Nachtrag an dieses Dokument wird angehaengt, nicht eingearbeitet, mit der Byte-Integritaet
des entschiedenen Bereichs, wie `02-DECISION-probe-budget.md:131-137` es vormacht.

---

# Nachtrag vom 2026-09-05

*Angehaengt von Plan 02-31, nicht eingearbeitet. Alles oberhalb dieser Ueberschrift ist die
Entscheidungsvorlage in dem Zustand, in dem Runde 7 sie vorgefunden hat, und ist byteweise
unveraendert: die ersten 34626 Bytes dieser Datei hashen weiterhin auf
`16d106e88b5e3b3ceafe1c80be6c1cbe89972729fd23951b4fceb6db1a75b5b1`. Nachgemessen mit*

```
head -c 34626 .planning/phases/02-transport-seam-talossim-image-factory/02-DECISION-drift-guard-lexik.md | shasum -a 256
```

*Dieser Abschnitt ist ein Anhang und nichts anderes. Insbesondere bleibt der Statusblock oben
unveraendert: `offen` — `vorgelegt, nicht ratifiziert`.*

## Die Provenienz: delegiert, nicht ratifiziert

**Die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf
delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert.**

Konkret: der Betreiber hat den Entscheidungs-Checkpoint von Plan 02-30 gelesen und mit *"entscheide
du"* geantwortet. Der Lauf hat daraufhin Variante B gewaehlt — auf diesen Gruenden: es ist die
Empfehlung des Panels; die Plaene 02-30 und 02-31 waren bereits fuer B geschrieben, waehrend A eine
Neuplanung verlangt haette; und zwei Pruef-Linsen hatten Variante A's Lexer gebaut und besiegt (eine
Tabelle als JSX-Textknoten in einem `<pre>`-Block, und der in `02-REVIEW.md:164-180` entworfene
`stripNonCode` gegen eine `${…}`-Interpolation).

**Der Abschnitt "Ratifikation" oben sieht vor, dass eine Ratifikation in den Statusblock dieses
Dokuments und in den Text von Eintrag 72 wandert. Das ist nicht der eingetretene Fall.** Eine
delegierte Wahl als Ratifikation zu verbuchen waere eine Behauptung, die breiter ist als ihre
Messung — genau der Defekt, den diese Runde schliessen soll, begangen von der Runde, die ihn
schliesst. Der Statusblock bleibt deshalb unangetastet, und **Ledger-Eintrag 72** (`unmet-truth`,
`open`, ohne Zeilennummer, zeigt auf `refusedRangesDecl`) traegt die Formulierung oben woertlich.

Ebenfalls festzuhalten, damit ein spaeterer Leser die Beweisdichte dieser Runde richtig einordnet:
**der Betreiber hat fuer den Rest dieser Runde ausdruecklich um schnelleres Vorgehen mit weniger
Verifikation gebeten.** Die Evidenz der Runde 7 ist duenner als die der Runden 5 bis 7 zuvor; es
wurde gemessen, was die `<verify>`-Bloecke und `<acceptance_criteria>` der Plaene verlangen, und
nichts darueber hinaus.

## Welcher Plan welchen Teil traegt

| Teil der Empfehlung | Plan | richtungsabhaengig? |
|---|---|---|
| Spiegel-Defekt ueber die Form des Rumpfes beseitigt (`strings.TrimSpace(body)`), Leer- und Abschneide-Zweig mit je eigener Ursache | 02-29 | nein — unbedingter Teil |
| WR-03: `to > utf8.MaxRune` und `from > to` als zwei Zweige mit zwei Ursachen | 02-29 | nein |
| WR-04: die drei `wantErr: "REFUSED_RANGES"`-Zeilen auf unterscheidende Teilzeichenketten gehoben | 02-29 | nein |
| WR-05: der 49-zeilige Kommentarblock geteilt, jede Regexp-Deklaration mit eigenem Doc-Kommentar | 02-29 | nein |
| Duplikatspruefung: mehr als eine `REFUSED_RANGES`-Deklaration ist ein eigener Fehlerausgang | 02-29 | nein — beim Planen ueber Punkt 6 gefunden |
| Der allquantifizierte Satz aus Doc-Kommentar und Fehlertext entfernt; Behauptung aus `guardBlindSpots` gerendert (`honestClaim`) | 02-30 | **ja — B** |
| Blindheit nach **Mechanismus** skopiert, nicht als "non-code" | 02-30 | **ja — B** |
| Blindheitstabellen ueber den Live-Lesepfad (`browserRefusalRanges(t, path)`, wrapper-frei) | 02-30 | **ja — B** |
| Fuenfter Ausschluss `surrogate-interior`, als `rowless` mit Grund gefuehrt | 02-30 | **ja — B** |
| Punkt 6 einmal ausgefuehrt und protokolliert (Ergebnis: kein Fund) | 02-30 | **ja — B** |
| IN-05: Eintrag 72 zeigt auf ein Symbol, Zeilenspalte leer; 70s Zeiger auf Zeile 83 damit erledigt | 02-31 | nein |
| Ledger-Eintrag 72, amendierend, `open`, mit der sechspunktigen Schliessbedingung | 02-31 | **ja — B** |
| `overrides:` nicht benutzt, `waived_count` bleibt 0 | 02-31 | nein |

Was ein Wechsel auf **Variante A** heute kostet: die Arbeit von Plan 02-30 an
`internal/imagefactory/guard_drift_test.go` waere neu zu planen, und Eintrag 72 beschriebe einen
anderen Stand — seine Abschnitte ueber die Ausschlussliste entfielen zugunsten einer Beschreibung
dessen, was ein Lexer schliesst und was nicht. Der unbedingte Teil (Plan 02-29 vollstaendig, plus
IN-05) bliebe unberuehrt. Der Satz aus dem Abschnitt "Ratifikation", der Wechsel koste *"einen
Satz"*, galt vor der Ausfuehrung; seit dem 2026-09-05 kostet er zusaetzlich einen Plan.

## Zahlenkorrektur: 2048 → 2046

Der Abschnitt "Was B in Runde 7 anders bekommen muss" schreibt oben *"Fuer 2048 Codepoints wird
nichts verglichen"*. **Nachgemessen ist die Zahl um zwei zu hoch.** U+D800..U+DFFF sind 2048
Codepoints, aber der Waechter behauptet **beide Endpunkte einzeln**, bevor der Sweep den Bereich mit
einem `continue` ueberspringt. Verglichen wird also nichts fuer die **2046** Codepoints *dazwischen*.

**Es gilt 2046.** Die Plaene 02-29, 02-30 und 02-31 dieser Runde, der `surrogate-interior`-Eintrag in
`guardBlindSpots` und der Text von Ledger-Eintrag 72 tragen durchgaengig 2046. Der Satz im Koerper
dieses Dokuments **bleibt unveraendert stehen** — korrigiert wird durch Anhaengen, nicht durch
Einarbeiten, genau wie beim Ledger, der ebenfalls kein Aenderungs-Verb kennt. Diese Notiz existiert,
damit ein Leser, der von Eintrag 72 auf dieses Dokument weitergeht, nicht auf einen Widerspruch
stoesst und die genauere Zahl fuer einen Tippfehler haelt.

## Was dieser Nachtrag nicht tut

Er ratifiziert nichts, er hebt den Statusblock nicht, und er schliesst Eintrag 70 nicht. **Eintrag 70
bleibt `open`**, Eintrag 72 amendiert ihn als neuer Eintrag, und **G-02-9 ist offen** — die Eintraege
58 und 66 tragen ihn unberuehrt. Eintrag 56 (die SERVER-Menge ist nicht erschoepfend gegen
`factory.talos.dev` gemessen) wird von keiner Variante um ein Byte besser und bleibt daneben offen.
