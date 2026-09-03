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
# Das Repository ist privat, also braucht der Download einen Token. Er liegt in
# TOKEN_FILE, gehoert root und wird nur von diesem Skript gelesen. Ein
# fine-grained PAT mit "Contents: read" auf genau dieses Repository reicht -
# mehr Rechte braucht es nicht, und ein Token, das mehr kann, liegt hier ohne
# Grund.
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

# Woher die Releases kommen. Ein Repo, seit die beiden am 2026-09-03 wieder
# zusammengelegt wurden. CONF darf es ueberschreiben, ohne das Skript zu
# aendern:
#
#   echo 'REPO=holzcloud/anderes-repo' > /etc/holzkube-manager/update.conf
REPO=${REPO:-holzcloud/holzkube-manager}
CONF=/etc/holzkube-manager/update.conf
# shellcheck source=/dev/null
[[ -r $CONF ]] && . "$CONF"

SERVICE=holzkube-manager.service
BIN=/usr/local/bin/holzkube-managerd
PREVIOUS=/usr/local/lib/holzkube-manager/holzkube-managerd.previous
TOKEN_FILE=/etc/holzkube-manager/github-token
HEALTH_URL=https://127.0.0.1:8443/api/v1/system/status

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
    -h|--help)  sed -n '2,29p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "FEHLER: unbekannte Option $arg" >&2; exit 1 ;;
  esac
done

log()  { printf '%s\n' "$*"; }
fail() { printf 'FEHLER: %s\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || fail "muss als root laufen (sudo $0)"

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

command -v curl    >/dev/null || fail "curl fehlt"
command -v tar     >/dev/null || fail "tar fehlt"
command -v python3 >/dev/null || fail "python3 fehlt"

[[ -r $TOKEN_FILE ]] || fail "$TOKEN_FILE fehlt.
Lege dort einen fine-grained GitHub-Token mit 'Contents: read' auf $REPO ab:
  install -d -m 0700 -o root -g root /etc/holzkube-manager
  printf '%s' 'github_pat_...' > $TOKEN_FILE
  chmod 0600 $TOKEN_FILE"
TOKEN=$(tr -d ' \t\r\n' < "$TOKEN_FILE")
[[ -n $TOKEN ]] || fail "$TOKEN_FILE ist leer"

api() {
  # --fail-with-body, damit ein 404 als Fehler ankommt und trotzdem sagt, was
  # GitHub geantwortet hat. Ein stiller leerer Body waere hier die schlechteste
  # aller Rueckmeldungen.
  curl -sS --fail-with-body --max-time 30 \
    -H "Authorization: Bearer $TOKEN" \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "$@"
}

# --- Welches Release ist das neueste? ---------------------------------------
# /releases/latest ueberspringt Drafts und Prereleases, und das ist hier die
# richtige Semantik: goreleaser legt jedes Release als Draft an (release.draft
# in .goreleaser.yaml), und ein Draft ist ausdruecklich noch nicht freigegeben.
# Ein Update-Skript, das Drafts installiert, wuerde die Bedeutung des Wortes
# unterlaufen.
#
# Damit die Fehlermeldung trotzdem hilft, wird bei einem 404 in der vollen
# Liste nachgesehen und der wartende Draft beim Namen genannt.
if ! LATEST_JSON=$(api "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null); then
  DRAFTS=$(api "https://api.github.com/repos/$REPO/releases?per_page=10" 2>/dev/null \
    | python3 -c "
import json,sys
try: rs = json.load(sys.stdin)
except Exception: sys.exit(0)
print(', '.join(r['tag_name'] for r in rs if r.get('draft')))
" || true)
  if [[ -n ${DRAFTS:-} ]]; then
    fail "kein veroeffentlichtes Release. Als Entwurf liegt bereit: $DRAFTS
goreleaser legt Releases als Draft an - auf GitHub freigeben, dann erneut ausfuehren."
  fi
  fail "kein Release in $REPO gefunden.
Ein Release entsteht durch einen Tag: git tag -a v1.0.0 -m 'v1.0.0' && git push origin v1.0.0
Der CI-Job 'release' baut daraus die Archive."
fi

read -r TAG ASSET_ID ASSET_NAME SUMS_ID < <(printf '%s' "$LATEST_JSON" | python3 -c "
import json,sys
r = json.load(sys.stdin)
want = 'linux_${ARCH}.tar.gz'
asset = next((a for a in r['assets'] if a['name'].endswith(want)), None)
sums  = next((a for a in r['assets'] if a['name'] == 'checksums.txt'), None)
if asset is None:
    sys.exit('kein Asset fuer ' + want + ' in ' + r['tag_name'])
print(r['tag_name'], asset['id'], asset['name'], sums['id'] if sums else '')
") || fail "Release-Metadaten nicht lesbar"

REMOTE_VERSION=${TAG#v}
LOCAL_VERSION=$("$BIN" --version 2>/dev/null | awk '{print $NF}' || echo "keine")

log "installiert: $LOCAL_VERSION"
log "neuestes Release: $TAG ($ASSET_NAME)"

if [[ $CHECK_ONLY -eq 1 ]]; then
  [[ $LOCAL_VERSION == "$REMOTE_VERSION" ]] && log "aktuell." || log "Update verfuegbar."
  exit 0
fi

if [[ $LOCAL_VERSION == "$REMOTE_VERSION" && $FORCE -eq 0 ]]; then
  log "Bereits aktuell. --force installiert trotzdem neu."
  exit 0
fi

# --- Herunterladen und pruefen ----------------------------------------------
TMP=$(mktemp -d /tmp/holzkube-manager-update.XXXXXX)
trap 'rm -rf "$TMP"' EXIT

log "lade $ASSET_NAME ..."
# Accept: octet-stream liefert die Datei statt der Metadaten. GitHub leitet auf
# einen Speicher-Host um; curl schickt den Authorization-Header ueber eine
# Host-Grenze hinweg nicht mit, was hier genau richtig ist - der Token hat auf
# dem Speicher-Host nichts zu suchen und wuerde dort einen 400 ausloesen.
api -L -H "Accept: application/octet-stream" \
  -o "$TMP/$ASSET_NAME" \
  "https://api.github.com/repos/$REPO/releases/assets/$ASSET_ID" \
  || fail "Download fehlgeschlagen"

if [[ -n $SUMS_ID ]]; then
  api -L -H "Accept: application/octet-stream" \
    -o "$TMP/checksums.txt" \
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
for _ in $(seq 1 20); do
  if systemctl is-active --quiet "$SERVICE" \
     && curl -sk --max-time 5 "$HEALTH_URL" | grep -q '"audit_chain"'; then
    ok=1; break
  fi
  sleep 1
done

if [[ $ok -eq 1 ]]; then
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
  log "Zurueckgerollt auf $("$BIN" --version)."
fi
exit 1
