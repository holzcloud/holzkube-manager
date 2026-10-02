#!/usr/bin/env bash
#
# Aktualisiert holzkube-managerd auf dem Host aus dem neuesten GitHub-Release.
#
#   sudo holzkube-manager-update           # aktualisieren, wenn es etwas Neues gibt
#   sudo holzkube-manager-update --check   # nur nachsehen, nichts anfassen
#   sudo holzkube-manager-update --force   # auch neu installieren, wenn die Version gleich ist
#   sudo holzkube-manager-update --rollback
#
# Voraussetzungen auf dem Host: curl, tar, python3, systemd. Alle sind auf einem
# Debian-Standardsystem vorhanden; jq bewusst nicht, weil es das nicht ist.
#
# Und der Aufbau, fuer den das Release gebaut ist: der Daemon als
# /usr/local/bin/holzkube-managerd, gestartet von holzkube-manager.service,
# erreichbar unter https://127.0.0.1:8443 (das voreingestellte --listen, oder
# 0.0.0.0:8443). Ein Lauf, der installieren darf, prueft das, bevor er
# irgendetwas laedt, und verweigert sich sonst mit einem Grund (docs/guide.md,
# "Running it as a service").
#
# Ist das Repository oeffentlich, braucht es nichts weiter. Ist es privat, legt
# man einen fine-grained PAT mit "Contents: read" auf genau dieses Repository
# nach /etc/holzkube-manager/github-token, root-only 0600 - mehr Rechte braucht
# es nicht, und ein Token, das mehr kann, liegt dort ohne Grund. Das Skript
# merkt selbst, welcher Fall vorliegt.
#
# Was dieses Skript ausdruecklich NICHT anfasst:
#
#   - Das Datenverzeichnis. Konto, Sessions, Audit-Log und das TLS-Zertifikat
#     bleiben, wo sie sind.
#   - Das Zertifikat. Es wird einmal erzeugt und danach wiederverwendet, und
#     genau darauf verlaesst sich die BackendTLSPolicy im Cluster, die es
#     gepinnt hat. Ein Update, das ein neues Zertifikat erzeugte, wuerde
#     manager.example.com mit 503 beantworten, bis der ConfigMap-Block im
#     Infra-Repo ersetzt ist.
#   - Die Unit. Konfiguration bleibt Konfiguration; dieses Skript tauscht ein
#     Binary aus.
set -euo pipefail

# Wo dieses Skript liegt, bevor es das Verzeichnis wechselt: $0 darf relativ
# sein (sudo ./holzkube-manager-update.sh), und nach dem cd zeigte es ins Leere
# -- oder, schlimmer, auf eine andere Datei, die sich nach einem gesunden Update
# mit dem neuen Skript ueberschreiben liesse.
SELF=$(readlink -f "$0")
# Nichts hier haengt vom Verzeichnis des Aufrufers ab, und es laeuft als root:
# wer `sudo holzkube-manager-update` in /tmp oder einem fremden Checkout
# aufruft, soll dort nichts ausfuehren, was ein anderer hingelegt hat. python3
# laeuft zusaetzlich ueberall mit -I (isoliert): ohne das stellt `python3 -`
# und `python3 -c` das aktuelle Verzeichnis an den Anfang von sys.path und
# liest PYTHONPATH, und ein json.py oder datetime.py dort liefe als root.
cd /

# Pfade, die die Umgebung ueberschreiben darf. Im Betrieb setzt sie niemand:
# die Unit nicht, und sudo verwirft sie mit env_reset. Es gibt sie fuer den
# Test in internal/host/updatestatus, der eine Kopie dieses Skripts gegen
# Attrappen laufen laesst und dabei weder die Konfiguration noch das Binary
# dieses Hosts anfassen darf:
#
#   HOLZKUBE_MANAGER_UPDATE_CONF        /etc/holzkube-manager/update.conf
#   HOLZKUBE_MANAGER_SERVICE            holzkube-manager.service
#   HOLZKUBE_MANAGER_BIN                /usr/local/bin/holzkube-managerd
#   HOLZKUBE_MANAGER_PREVIOUS           /usr/local/lib/holzkube-manager/holzkube-managerd.previous
#   HOLZKUBE_MANAGER_TOKEN_FILE         /etc/holzkube-manager/github-token
#   HOLZKUBE_MANAGER_HEALTH_URL         https://127.0.0.1:8443/api/v1/system/status
#   HOLZKUBE_MANAGER_UPDATE_STATUS_DIR  /var/lib/holzkube-manager-update
#   HOLZKUBE_MANAGER_UPDATE_LOCK_WAIT   60 mit --check, sonst 600 (Sekunden)
#
# update.conf wird unten gelesen, bevor diese Werte gelten, und darf darum
# zwei davon fuer einen Host setzen, der anders aufgebaut ist:
# HOLZKUBE_MANAGER_SERVICE (die Unit des Daemons heisst anders) und
# HOLZKUBE_MANAGER_HEALTH_URL (der Daemon lauscht nicht auf 127.0.0.1:8443).
# Die Pfade nicht: holzkube-manager-update.service darf nur die
# voreingestellten schreiben.

# Woher die Releases kommen. Ein Repo, seit die beiden am 2026-09-03 wieder
# zusammengelegt wurden. CONF darf es ueberschreiben, ohne das Skript zu
# aendern:
#
#   echo 'REPO=holzcloud/anderes-repo' > /etc/holzkube-manager/update.conf
REPO=${REPO:-holzcloud/holzkube-manager}
CONF=${HOLZKUBE_MANAGER_UPDATE_CONF:-/etc/holzkube-manager/update.conf}
# shellcheck source=/dev/null
[[ -r $CONF ]] && . "$CONF"

SERVICE=${HOLZKUBE_MANAGER_SERVICE:-holzkube-manager.service}
BIN=${HOLZKUBE_MANAGER_BIN:-/usr/local/bin/holzkube-managerd}
PREVIOUS=${HOLZKUBE_MANAGER_PREVIOUS:-/usr/local/lib/holzkube-manager/holzkube-managerd.previous}
TOKEN_FILE=${HOLZKUBE_MANAGER_TOKEN_FILE:-/etc/holzkube-manager/github-token}
HEALTH_URL=${HOLZKUBE_MANAGER_HEALTH_URL:-https://127.0.0.1:8443/api/v1/system/status}
STATUS_DIR=${HOLZKUBE_MANAGER_UPDATE_STATUS_DIR:-/var/lib/holzkube-manager-update}

# Die Architektur wird nicht angenommen, sondern gelesen: dasselbe Skript soll
# auf dem Pi und auf einem amd64-Host dasselbe tun.
case "$(uname -m)" in
  aarch64|arm64) ARCH=arm64 ;;
  x86_64|amd64)  ARCH=amd64 ;;
  *) echo "FEHLER: keine Release-Architektur fuer $(uname -m)" >&2; exit 1 ;;
esac

CHECK_ONLY=0
FORCE=0
ROLLBACK=0
for arg in "$@"; do
  case "$arg" in
    --check)    CHECK_ONLY=1 ;;
    --force)    FORCE=1 ;;
    --rollback) ROLLBACK=1 ;;
    -h|--help)  awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$SELF"; exit 0 ;;
    *) echo "FEHLER: unbekannte Option $arg" >&2; exit 1 ;;
  esac
done

log()  { printf '%s\n' "$*"; }
fail() { printf 'FEHLER: %s\n' "$*" >&2; exit 1; }

# --- Was zuletzt geschah ----------------------------------------------------
# Nach jedem Lauf, der zu einer Entscheidung kommt, steht in
# $STATUS_DIR/status.json, wann nachgesehen wurde, was installiert war, was das
# neueste Release war und was daraus wurde: current, available, updated,
# rolled-back oder failed. Die Host-Seite des Daemons liest genau diese Datei
# (internal/host/updatestatus) - sonst weiss dort niemand, was dieser Timer
# stuendlich tut.
#
# Das Verzeichnis gehoert root und liegt ausdruecklich nicht im
# Datenverzeichnis des Daemons: das gehoert dem Dienstbenutzer, und root, das in
# ein Verzeichnis schreibt, das ein anderer kontrolliert, schreibt dorthin, wo
# dessen Symlink hinzeigt.
#
# Das Festhalten ist Nebensache. Es darf kein Update scheitern lassen und
# keinen Exit-Code aendern; darum laeuft es aus genau einer EXIT-Falle, mit
# set +e und || true, und gibt bei jedem Hindernis still auf.
#
# Die Variablen werden hier geleert, nicht aus der Umgebung uebernommen: TMP
# zumal ist in manchen Umgebungen gesetzt (TMP=/tmp), und die Falle loescht,
# was darin steht.
OUTCOME=""
INSTALLED=""
LATEST=""
TMP=""

# status_dir_ok sagt, ob in $STATUS_DIR geschrieben werden darf, und legt es als
# root an, wenn es fehlt. Dieselbe Frage stellen das Festhalten und die Sperre
# unten: wer festhalten darf, sperrt, und wer nicht festhalten darf, braucht
# keine Sperre.
status_dir_ok() {
  local dir=${STATUS_DIR:-}
  [[ -n $dir ]] || return 1
  # Ein Symlink wird nicht verfolgt, von niemandem.
  [[ ! -L $dir ]] || return 1
  if [[ ! -d $dir ]]; then
    # Anlegen darf es nur root, und dann so, wie es gehoert.
    [[ $EUID -eq 0 ]] || return 1
    install -d -o root -g root -m 0755 "$dir" || return 1
  fi
  # Als root nur in ein Verzeichnis, das root gehoert und in das niemand sonst
  # schreiben kann. Gehoert es root, ist aber gruppen- oder weltschreibbar,
  # kann ein anderer die Datei von mktemp zwischen mktemp und der Umleitung
  # unten gegen einen Symlink tauschen, und root schriebe dorthin, wo er zeigt
  # -- auch mit Sticky-Bit, denn die Datei, die er tauschte, waere dann seine.
  # Die Pruefung laeuft in python3, weil stat(1) auf Linux und BSD
  # verschiedene Schalter hat, und mit lstat, damit sie keinem Symlink folgt.
  if [[ $EUID -eq 0 ]]; then
    python3 -I -c 'import os, stat, sys; s = os.lstat(sys.argv[1]); sys.exit(0 if stat.S_ISDIR(s.st_mode) and s.st_uid == 0 and not s.st_mode & 0o022 else 1)' "$dir" \
      || return 1
  fi
  # --check ohne root: nicht schreibbar heisst nichts festhalten.
  [[ -w $dir ]]
}

record_status() {
  local rc=$1 outcome=${OUTCOME:-} dir=${STATUS_DIR:-} tmp
  if [[ -z $outcome && $rc -ne 0 ]]; then
    outcome=failed
  fi
  # Nur ein gescheiterter Lauf darf eine Version offen lassen; so liest es
  # internal/host/updatestatus, und eine Datei, die der Leser verweigert,
  # zeigte die Seite als "nicht lesbar" statt als das, was geschah. Wer nicht
  # sagen kann, was installiert ist, hat nicht "aktualisiert".
  if [[ -n $outcome && $outcome != failed && ( -z ${INSTALLED:-} || -z ${LATEST:-} ) ]]; then
    outcome=failed
  fi
  [[ -n $outcome && -n $dir ]] || return 0
  status_dir_ok || return 0

  tmp=$(mktemp "$dir/.status.XXXXXX") || return 0
  # Die Werte gehen als argv hinein und als JSON heraus. Tag-Namen kommen von
  # GitHub; sie in JSON-Text einzusetzen hiesse, GitHub die Syntax der Datei
  # bestimmen zu lassen.
  if ! python3 -I - "$outcome" "${INSTALLED:-}" "${LATEST:-}" > "$tmp" <<'PY'
import datetime, json, sys
outcome, installed, latest = sys.argv[1:4]
json.dump({
    "checked_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "installed": installed or None,
    "latest": latest or None,
    "outcome": outcome,
}, sys.stdout)
sys.stdout.write("\n")
PY
  then
    rm -f "$tmp"
    return 0
  fi
  if ! { chmod 0644 "$tmp" && mv -f "$tmp" "$dir/status.json"; }; then
    rm -f "$tmp"
  fi
  return 0
}

# Die einzige EXIT-Falle. Sie raeumt das Arbeitsverzeichnis weg und haelt fest,
# was geschah. Sie ruft nie exit, also bleibt der Exit-Code der, den das Skript
# ohnehin hatte; und set +e, weil unter set -e jeder scheiternde Befehl in der
# Falle das Skript mit seinem eigenen Status beenden wuerde.
on_exit() {
  local rc=$?
  set +e
  if [[ -n ${TMP:-} ]]; then
    rm -rf "$TMP"
  fi
  # Festgehalten wird, was jetzt auf der Platte liegt, nicht was vor dem Lauf
  # dort lag: ein Lauf, der nach dem install scheitert oder abgebrochen wird,
  # hat das neue Binary schon hingelegt, und ein Rollback das vorige zurueck.
  # Antwortet es nicht, ist die Version unbekannt -- leer, nicht die alte.
  INSTALLED=$("$BIN" --version 2>/dev/null | awk '{print $NF}')
  record_status "$rc" >/dev/null 2>&1 || true
}

# root braucht nur, wer etwas veraendert. --check sieht nach und fasst nichts
# an; es dafuer sudo zu verlangen, erzieht dazu, alles mit sudo aufzurufen.
if [[ $CHECK_ONLY -eq 0 ]]; then
  [[ $EUID -eq 0 ]] || fail "muss als root laufen (sudo $0)"
fi

# --- Rollback ---------------------------------------------------------------
# Steht vor allem anderen, weil es der Pfad ist, den jemand unter Zeitdruck
# sucht: kein Netz, kein Token, keine API.
if [[ $ROLLBACK -eq 1 ]]; then
  [[ -x $PREVIOUS ]] || fail "keine vorherige Version unter $PREVIOUS"
  log "Zurueck auf: $("$PREVIOUS" --version)"
  install -o root -g root -m 0755 "$PREVIOUS" "$BIN"
  systemctl restart "$SERVICE"
  log "Zurueckgerollt. Die Datei unter $PREVIOUS bleibt liegen."
  exit 0
fi

# --- Ein Lauf nach dem anderen ----------------------------------------------
# Der stuendliche Lauf (holzkube-manager-update.service) und das Nachsehen
# (holzkube-manager-update-check.service, das der Host-Helfer startet) fuehren
# dieses Skript aus, und nichts ordnet die beiden Units gegeneinander. Ohne
# Sperre sah ein Nachsehen vor dem Installieren nach, hielt danach fest und
# ueberschrieb "updated" mit "available" -- fuer ein Release, das da schon
# installiert war (13-REVIEW-2 IN-02).
#
# Also nimmt jeder Lauf, der festhalten darf, zuerst eine Sperre: flock(1) auf
# $STATUS_DIR/.lock. Was das sicher macht:
#
#   - Die Sperre gehoert dem Kernel, nicht der Datei. Sie endet mit dem
#     letzten Prozess, der die Datei offen haelt, auch bei SIGKILL oder einem
#     Absturz; eine liegengebliebene .lock sperrt nichts. Die Kinder dieses
#     Skripts erben den Deskriptor; das laengste ist curl mit --max-time 300.
#   - Gewartet wird begrenzt: mit --check 60 s, damit Warten und Nachsehen
#     (curl --max-time 30) unter TimeoutStartSec=2min der Check-Unit bleiben;
#     sonst 600 s, mehr als das Nachsehen je dauert (die Unit beendet es nach
#     hoechstens 2min40s). Bleibt sie laenger gehalten, endet der Lauf mit 1
#     und haelt nichts fest: festhalten wird der, der sie haelt, und ein
#     Eintrag daneben waere genau das Rennen. Der Timer startet die naechste
#     Stunde wie immer.
#   - Die Datei ist 0600 und gehoert root. Lesen kann sie sonst niemand, also
#     kann auch niemand anderes sie sperren -- der Dienstbenutzer des Daemons,
#     der status.json liest, haelt so kein Update auf.
#   - Was die Sperre selbst verhindert, haelt kein Update auf: fehlt flock, ist
#     das Verzeichnis nicht verwendbar, die Datei nicht zu oeffnen oder scheitert
#     flock an etwas anderem als einer gehaltenen Sperre, laeuft das Skript wie
#     vorher ohne. Nur eine Sperre, die ein anderer Lauf
#     wirklich haelt, laesst es warten.
#
# Ohne Sperre bleiben --help, eine unbekannte Option, die Weigerung ohne root
# und --rollback: sie enden oben und halten nichts fest. Ein --check ohne root
# darf nicht festhalten und sperrt darum auch nicht.
take_lock() {
  local wait=$1 lock old
  command -v flock >/dev/null || { log "WARNUNG: flock fehlt - dieser Lauf laeuft ohne Sperre."; return 0; }
  status_dir_ok >/dev/null 2>&1 || return 0
  lock=$STATUS_DIR/.lock
  [[ ! -L $lock ]] || return 0
  old=$(umask)
  umask 077
  if ! exec 9>>"$lock"; then
    umask "$old"
    return 0
  fi
  umask "$old"
  # Nur 75 heisst "gehalten" (-E). Jeder andere Fehler von flock ist einer
  # von flock selbst und kein anderer Lauf; dann geht es ohne Sperre weiter,
  # statt jede Stunde zu warten und aufzugeben.
  local rc=0
  flock -n -E 75 9 || rc=$?
  case $rc in
    0)  return 0 ;;
    75) ;;
    *)  log "WARNUNG: flock endete mit $rc - dieser Lauf laeuft ohne Sperre."; return 0 ;;
  esac
  log "Ein anderer Lauf des Update-Skripts laeuft; warte hoechstens $wait s auf ihn ..."
  rc=0
  flock -w "$wait" -E 75 9 || rc=$?
  case $rc in
    0)  return 0 ;;
    75) fail "ein anderer Lauf des Update-Skripts haelt $lock seit ueber $wait s.
Dieser Lauf hat nichts nachgesehen, nichts installiert und nichts festgehalten." ;;
    *)  log "WARNUNG: flock endete mit $rc - dieser Lauf laeuft ohne Sperre."; return 0 ;;
  esac
}

LOCK_WAIT=${HOLZKUBE_MANAGER_UPDATE_LOCK_WAIT:-}
if [[ ! $LOCK_WAIT =~ ^[0-9]+$ ]]; then
  LOCK_WAIT=600
  [[ $CHECK_ONLY -eq 1 ]] && LOCK_WAIT=60
fi
take_lock "$LOCK_WAIT"

# Ab hier kommt jeder Lauf zu einer Entscheidung, und die wird festgehalten.
# --help, eine unbekannte Option, die Weigerung ohne root und --rollback enden
# oben und halten nichts fest; ein Lauf, der die Sperre nicht bekam, auch nicht.
trap on_exit EXIT
# Ein Signal beendet den Lauf ueber exit, mit 128 + Signalnummer. Ohne diese
# Fallen laeuft die EXIT-Falle bei SIGTERM zwar auch, sieht in $? aber den
# Status des letzten fertigen Befehls, meist 0 -- und ein Lauf, den
# `systemctl stop`, ein Herunterfahren oder Strg-C mitten im Update beendet,
# hielte nichts fest, waehrend die Seite den vorigen Lauf weiter anzeigt.
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

# Was vor dem Lauf installiert ist, fuer den Vergleich mit dem Release. Was
# festgehalten wird, liest on_exit erst am Ende neu.
LOCAL_VERSION=$("$BIN" --version 2>/dev/null | awk '{print $NF}' || echo "keine")

command -v curl    >/dev/null || fail "curl fehlt"
command -v tar     >/dev/null || fail "tar fehlt"
command -v python3 >/dev/null || fail "python3 fehlt"

# healthy fragt einmal, ob der Dienst laeuft und unter $HEALTH_URL antwortet.
healthy() {
  systemctl is-active --quiet "$SERVICE" \
    && curl -sk --max-time 5 "$HEALTH_URL" | grep -q '"audit_chain"'
}

# wait_healthy fragt hoechstens $1 Mal, mit einer Sekunde Pause dazwischen.
wait_healthy() {
  local tries=$1
  for _ in $(seq 1 "$tries"); do
    healthy && return 0
    sleep 1
  done
  return 1
}

# --- Passt dieser Host zu dem, was das Skript ersetzt? ----------------------
# Ein Update ersetzt $BIN und startet $SERVICE neu, und "updated" oder
# "current" ist nur dort wahr, wo $SERVICE genau $BIN startet und unter
# $HEALTH_URL antwortet. Sonst installierte das Skript ein Binary, das niemand
# startet, und hielte es ab der naechsten Stunde fuer aktuell; oder es hielte
# "updated" fest, waehrend der alte Prozess weiterlaeuft; oder es rollte jede
# Stunde zurueck, weil die Pruefung eine Adresse fragt, auf der niemand
# lauscht (13-REVIEW-3 CR-01). Darum fragt jeder Lauf, der installieren darf,
# zuerst -- ohne Netz, ohne etwas anzufassen -- und verweigert sich sonst: das
# haelt "failed" fest, und hier steht, warum.
#
# Gefragt wird, was der Neustart ausfuehren wird: ExecStart= der geladenen
# Unit, wie PID 1 sie kennt. Ein Pfad, der ueber einen Symlink auf $BIN zeigt,
# zaehlt; ein Wrapper, ein anderer Pfad oder mehr als ein ExecStart= nicht.
# Laeuft der Dienst gerade nicht, wird er vorher nicht gefragt: das Update
# kann das sein, was ihn zurueckbringt, und die Pruefung nach dem Neustart
# entscheidet.
#
# --check fragt das nicht: es installiert nichts und startet nichts neu.
check_host() {
  local load exec_paths resolved
  load=$(systemctl show --value -p LoadState "$SERVICE" 2>/dev/null) || load=""
  [[ $load == loaded ]] || fail "$SERVICE ist hier nicht installiert (LoadState=${load:-unbekannt}).
Dieses Skript ersetzt $BIN und startet $SERVICE neu. Ohne diese Unit laege ein
neues Binary da, das niemand startet, und die naechste Stunde hielte es fuer
aktuell. Wie der Daemon als $SERVICE laeuft, sagt docs/guide.md, \"Running it
as a service\"; heisst seine Unit anders, gehoert ihr Name als
HOLZKUBE_MANAGER_SERVICE nach $CONF. Nichts geladen, nichts installiert."

  exec_paths=$(systemctl show --value -p ExecStart "$SERVICE" 2>/dev/null \
    | sed -n 's/^{ path=\([^ ;]*\) ;.*$/\1/p') || exec_paths=""
  if [[ -z $exec_paths || $exec_paths == *$'\n'* ]]; then
    fail "$SERVICE hat nicht genau ein ExecStart=, das sich lesen laesst.
Ein Update ersetzt $BIN; ob der Neustart das ausfuehrt, ist so nicht zu sagen.
Nichts geladen, nichts installiert."
  fi
  resolved=$(readlink -f -- "$exec_paths" 2>/dev/null) || resolved=""
  [[ $exec_paths == "$BIN" || $resolved == "$BIN" ]] || fail "$SERVICE startet $exec_paths, nicht $BIN.
Ein Update ersetzte $BIN, der Neustart liesse $exec_paths laufen, und die
Pruefung danach saehe den alten Prozess als das neue Release. Nichts geladen,
nichts installiert."

  if systemctl is-active --quiet "$SERVICE"; then
    wait_healthy 3 || fail "$SERVICE laeuft, antwortet aber nicht unter $HEALTH_URL.
Nach einem Update scheiterte dieselbe Pruefung, und jede Stunde rollte zurueck.
Lauscht der Daemon nicht auf 127.0.0.1:8443 (--listen), gehoert die Adresse,
unter der er antwortet, als HOLZKUBE_MANAGER_HEALTH_URL nach $CONF. Nichts
geladen, nichts installiert."
  else
    log "$SERVICE laeuft gerade nicht; ob es unter $HEALTH_URL antwortet, zeigt der Neustart."
  fi
}

if [[ $CHECK_ONLY -eq 0 ]]; then
  check_host
fi

# Der Token ist optional, und zwar nach Lage des Repositories statt nach
# Konfiguration: ein oeffentliches Release laedt anonym, ein privates nicht. Die
# Datei wird gelesen, wenn sie da ist, und sonst nicht vermisst - so muss beim
# Umschalten von privat auf oeffentlich nichts nachgezogen werden.
TOKEN=""
if [[ -r $TOKEN_FILE ]]; then
  TOKEN=$(tr -d ' \t\r\n' < "$TOKEN_FILE")
  [[ -n $TOKEN ]] || fail "$TOKEN_FILE existiert, ist aber leer.
Entweder einen Token hineinschreiben oder die Datei loeschen - eine leere Datei
sieht aus wie eine Konfiguration und ist keine."
fi

# AUTH ist ein Array und keine Zeichenkette. ${TOKEN:+-H "Authorization: Bearer
# $TOKEN"} sieht richtig aus und ist es nicht: die Anfuehrungszeichen darin sind
# literale Zeichen, keine Quotierung, also zerfaellt der Header bei gesetztem
# Token in vier Woerter. Ein leeres Array expandiert dagegen zu nichts.
AUTH=()
[[ -n $TOKEN ]] && AUTH=(-H "Authorization: Bearer $TOKEN")

api() {
  # Metadaten. --fail-with-body, damit ein 404 als Fehler ankommt und trotzdem
  # sagt, was GitHub geantwortet hat. Ein stiller leerer Body waere hier die
  # schlechteste aller Rueckmeldungen.
  #
  # Ohne Token wird der Header weggelassen statt leer gesetzt: ein
  # "Authorization: Bearer " ohne Wert beantwortet GitHub mit 401, was dann wie
  # ein falscher Token aussaehe statt wie gar keiner.
  curl -sS --fail-with-body --max-time 30 \
    "${AUTH[@]}" \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "$@"
}

# download holt ein Release-Asset. Eine eigene Funktion, und das ist keine
# Kosmetik: der Asset-Endpunkt liefert die Datei nur bei
# "Accept: application/octet-stream" und sonst seine eigenen Metadaten. Als
# api() den JSON-Accept setzte und die Aufrufstelle den Octet-Stream-Accept
# dazuhaengte, schickte curl beide, GitHub bediente den ersten - und das Skript
# lud 1818 Bytes JSON, wo ein Tarball hingehoerte. Aufgefallen ist es nur, weil
# die Pruefsumme nicht passte; ohne sie waere der Fehler ein `tar`-Absturz nach
# dem Stoppen des Dienstes gewesen.
#
# Zwei Funktionen koennen sich nicht gegenseitig einen Header unterschieben.
download() {
  curl -sSL --fail-with-body --max-time 300 \
    "${AUTH[@]}" \
    -H "Accept: application/octet-stream" \
    "$@"
}

# --- Welches Release ist das neueste? ---------------------------------------
# Jedes Release ist ein Prerelease (seit 2026-09-26: das Produkt ist Alpha, und
# GitHub soll das auch so zeigen). /releases/latest ueberspringt Prereleases und
# fand darum ab da nichts mehr. Gelesen wird deshalb die Liste, die GitHub
# neueste zuerst liefert, und genommen wird das erste Release, das kein Entwurf
# ist. Ein Entwurf ist ausdruecklich noch nicht freigegeben; ein Prerelease ist
# hier der Normalfall.
RELEASES_JSON=$(api "https://api.github.com/repos/$REPO/releases?per_page=20" 2>/dev/null) \
  || fail "die Release-Liste von $REPO ist nicht lesbar"
LATEST_JSON=$(printf '%s' "$RELEASES_JSON" | python3 -I -c "
import json,sys
rs = json.load(sys.stdin)
published = [r for r in rs if not r.get('draft')]
if not published:
    drafts = ', '.join(r['tag_name'] for r in rs if r.get('draft'))
    sys.exit('kein veroeffentlichtes Release' + (' (als Entwurf: ' + drafts + ')' if drafts else ''))
json.dump(published[0], sys.stdout)
") || fail "kein Release in $REPO gefunden, das installiert werden koennte."

# Ein Release traegt seit v1.16 ZWEI Archive pro Architektur: den Daemon und
# holzkubectl, das sein eigenes bekommen hat, damit niemand einen Server mit
# eingebauter Oberflaeche laedt, um an ein Kommandozeilenwerkzeug zu kommen.
# Beide Namen enden auf "linux_<arch>.tar.gz", also hat die alte Auswahl -- das
# erste Asset, dessen Name so endet -- ab diesem Release eine Muenze geworfen.
# Traf sie holzkubectl, scheiterte das Update am tar weiter unten, mit der
# Meldung "Archiv enthaelt kein holzkube-managerd": wahr, und sie beschuldigt
# das Archiv statt die Auswahl. Der Dienst wird dabei nicht angefasst, der
# Schaden ist also ein Abend und kein Ausfall.
#
# Der Praefix ist das, was die beiden unterscheidet, und mehr als eine
# Uebereinstimmung ist ein Fehler und keine Auswahl: lieber abbrechen und den
# Grund nennen, als sich fuer eines von zweien zu entscheiden, ohne sagen zu
# koennen warum.
read -r TAG ASSET_ID ASSET_NAME SUMS_ID < <(printf '%s' "$LATEST_JSON" | python3 -I -c "
import json,sys
r = json.load(sys.stdin)
want = 'linux_${ARCH}.tar.gz'
hits = [a for a in r['assets']
        if a['name'].startswith('holzkube-manager_') and a['name'].endswith(want)]
sums = next((a for a in r['assets'] if a['name'] == 'checksums.txt'), None)
if not hits:
    sys.exit('kein holzkube-manager-Archiv fuer ' + want + ' in ' + r['tag_name'])
if len(hits) > 1:
    sys.exit('mehrdeutig: ' + ', '.join(a['name'] for a in hits))
asset = hits[0]
print(r['tag_name'], asset['id'], asset['name'], sums['id'] if sums else '')
") || fail "Release-Metadaten nicht lesbar"

REMOTE_VERSION=${TAG#v}
LATEST=$REMOTE_VERSION

log "installiert: $LOCAL_VERSION"
log "neuestes Release: $TAG ($ASSET_NAME)"

if [[ $CHECK_ONLY -eq 1 ]]; then
  if [[ $LOCAL_VERSION == "$REMOTE_VERSION" ]]; then
    OUTCOME=current
    log "aktuell."
  else
    OUTCOME=available
    log "Update verfuegbar."
  fi
  exit 0
fi

if [[ $LOCAL_VERSION == "$REMOTE_VERSION" && $FORCE -eq 0 ]]; then
  OUTCOME=current
  log "Bereits aktuell. --force installiert trotzdem neu."
  exit 0
fi

# --- Herunterladen und pruefen ----------------------------------------------
# Aufgeraeumt wird es von on_exit. Eine zweite EXIT-Falle hier wuerde die
# erste ersetzen, und ab dann hielte kein Lauf mehr fest, was geschah.
TMP=$(mktemp -d /tmp/holzkube-manager-update.XXXXXX)

log "lade $ASSET_NAME ..."
# GitHub leitet auf einen Speicher-Host um; curl schickt den
# Authorization-Header ueber eine Host-Grenze hinweg nicht mit, was hier genau
# richtig ist - der Token hat auf dem Speicher-Host nichts zu suchen und wuerde
# dort einen 400 ausloesen.
download -o "$TMP/$ASSET_NAME" \
  "https://api.github.com/repos/$REPO/releases/assets/$ASSET_ID" \
  || fail "Download fehlgeschlagen"

if [[ -n $SUMS_ID ]]; then
  download -o "$TMP/checksums.txt" \
    "https://api.github.com/repos/$REPO/releases/assets/$SUMS_ID" \
    || fail "checksums.txt nicht ladbar"

  # Nur die eine Zeile pruefen. `sha256sum -c` ueber die ganze Datei wuerde an
  # den Archiven scheitern, die hier gar nicht liegen, und ein Fehler waere
  # dann nicht mehr von einem echten Integritaetsproblem zu unterscheiden.
  EXPECTED=$(awk -v n="$ASSET_NAME" '$2 == n || $2 == "*" n {print $1}' "$TMP/checksums.txt")
  [[ -n $EXPECTED ]] || fail "keine Pruefsumme fuer $ASSET_NAME in checksums.txt"
  ACTUAL=$(sha256sum "$TMP/$ASSET_NAME" | awk '{print $1}')
  [[ $EXPECTED == "$ACTUAL" ]] || fail "Pruefsumme stimmt nicht.
  erwartet: $EXPECTED
  bekommen: $ACTUAL"
  log "Pruefsumme ok."
else
  # Kein stilles Weitermachen: dass nicht geprueft wurde, gehoert ins Protokoll.
  log "WARNUNG: das Release enthaelt keine checksums.txt - Integritaet ungeprueft."
fi

tar -xzf "$TMP/$ASSET_NAME" -C "$TMP" holzkube-managerd || fail "Archiv enthaelt kein holzkube-managerd"
chmod +x "$TMP/holzkube-managerd"

# Das neue Binary muss auf diesem Host ueberhaupt starten koennen. --version
# laeuft ohne Datenverzeichnis und ohne Netz und faengt die falsche
# Architektur und ein kaputtes Archiv ab, bevor der Dienst angefasst wird.
NEW_VERSION=$("$TMP/holzkube-managerd" --version 2>&1) || fail "das neue Binary laeuft hier nicht: $NEW_VERSION"
log "geladen: $NEW_VERSION"

# --- Installieren -----------------------------------------------------------
install -d -o root -g root -m 0755 "$(dirname "$PREVIOUS")"
if [[ -x $BIN ]]; then
  cp -p "$BIN" "$PREVIOUS"
  log "vorherige Version gesichert: $PREVIOUS"
fi

# install(1) ersetzt atomar. Der laufende Prozess haelt seine alte Inode, also
# passiert bis zum Neustart nichts.
install -o root -g root -m 0755 "$TMP/holzkube-managerd" "$BIN"
systemctl restart "$SERVICE"

# --- Nachsehen, ob es wirklich laeuft ---------------------------------------
# Ein Update, das den Dienst kaputtmacht und "fertig" meldet, ist schlimmer als
# eines, das scheitert: niemand sieht nach.
ok=0
wait_healthy 20 && ok=1

if [[ $ok -eq 1 ]]; then
  # Das Archiv traegt dieses Skript mit. Es ersetzt sich erst, nachdem der
  # Dienst gesund ist: ein neues Skript, das mit einem kaputten Update kaeme,
  # soll nicht auch noch den Rueckweg tragen muessen. So braucht eine Aenderung
  # am Update-Weg -- wie die Umstellung auf Prereleases -- nie wieder Handarbeit
  # auf dem Host.
  if tar -xzf "$TMP/$ASSET_NAME" -C "$TMP" deploy/holzkube-manager-update.sh 2>/dev/null; then
    if ! cmp -s "$TMP/deploy/holzkube-manager-update.sh" "$SELF"; then
      install -o root -g root -m 0755 "$TMP/deploy/holzkube-manager-update.sh" "$SELF"
      log "Update-Skript erneuert: $SELF"
    fi
  fi
  OUTCOME=updated
  log ""
  log "Aktualisiert auf $("$BIN" --version)."
  log "Zuruecknehmen mit: sudo $0 --rollback"
  exit 0
fi

log ""
log "Der Dienst ist nach dem Update nicht gesund geworden - rolle zurueck."
journalctl -u "$SERVICE" --no-pager -n 20 -o cat || true
if [[ -x $PREVIOUS ]]; then
  install -o root -g root -m 0755 "$PREVIOUS" "$BIN"
  systemctl restart "$SERVICE"
  # Ohne vorherige Version bleibt OUTCOME leer, und Exit 1 wird zu "failed".
  OUTCOME=rolled-back
  log "Zurueckgerollt auf $("$BIN" --version)."
fi
exit 1
