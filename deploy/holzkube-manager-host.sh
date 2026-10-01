#!/usr/bin/env bash
#
# Fuehrt genau einen Host-Auftrag aus, den holzkube-managerd abgelegt hat.
#
# Der Daemon laeuft ohne Rechte und bleibt so gehaertet, wie er ist. Er startet
# nichts selbst: fuer die fuenf Host-Aktionen seiner Host-Seite legt er eine
# Zeile in sein Datenverzeichnis,
#
#   /var/lib/holzkube-manager/host-order      <aktion> <id>
#
# und holzkube-manager-host.path startet dieses Skript als root, sobald die
# Datei da ist. Es kennt genau fuenf Auftraege und fuer jeden einen festen
# Befehl:
#
#   reboot            systemctl reboot
#   poweroff          systemctl poweroff
#   restart-service   systemctl restart holzkube-manager.service
#   update            systemctl start --no-block holzkube-manager-update.service
#   check-update      systemctl start holzkube-manager-update-check.service
#
# holzkube-managerd liest die folgende Zeile aus dem installierten Skript, um
# zu erfahren, welche Auftraege dieser Helfer ausfuehrt. Ein Helfer ohne sie
# kennt genau die ersten vier (reboot, poweroff, restart-service, update). Sie
# nennt genau die Woerter des Musters und des case weiter unten, jedes einmal;
# ein Test haelt sie dazu an.
# holzkube-manager-host orders: reboot poweroff restart-service update check-update
#
# Alles andere wird verworfen. Der Text eines Auftrags wird nie ausgefuehrt,
# nie ausgewertet, nie ungequotet benutzt und nie ins Journal geschrieben:
# protokolliert werden nur der Grund und die Laenge.
#
# Was dieses Skript ausdruecklich NICHT anfasst:
#
#   - Das Datenverzeichnis des Daemons, bis auf den Auftrag: es nimmt ihn per
#     rename(2) unter einen eigenen Namen im selben Verzeichnis und entfernt
#     ihn dort (weder rename noch rm folgen einem Symlink); es schreibt dort
#     nichts. Das Verzeichnis gehoert einem anderen Benutzer, und root, das
#     dort schriebe, schriebe dorthin, wo dessen Symlink hinzeigt.
#   - Irgendetwas ausser seinem eigenen Zustandsverzeichnis
#     (/var/lib/holzkube-manager-host, StateDirectory der Unit). Dort steht in
#     "last", was aus dem letzten Auftrag wurde:
#
#       <id> <aktion> started|rejected|failed <zeit>
#
#     Die Host-Seite liest genau diese Zeile (internal/host/hostaction).
#
# Exit-Codes: 0 kein Auftrag oder gestartet, 2 verworfen, 1 gescheitert.
set -euo pipefail

# Nichts hier haengt vom Verzeichnis des Aufrufers ab, und es laeuft als root.
cd /
# [0-9a-f] im Muster unten ist nur mit LC_ALL=C genau ASCII; in einer anderen
# Locale kann ein Bereich mehr Zeichen umfassen, als er zeigt.
export LC_ALL=C
# Ein fester PATH: kein Befehl hier soll von woanders kommen.
export PATH=/usr/sbin:/usr/bin:/sbin:/bin
# "last" wird 0644, damit der Daemon es lesen kann, und nichts mehr.
umask 022

# Pfade, die die Umgebung ueberschreiben darf. Die ausgelieferte Unit setzt
# keines davon.
#
#   HOLZKUBE_MANAGER_HOST_ORDER      /var/lib/holzkube-manager/host-order
#   HOLZKUBE_MANAGER_HOST_STATE_DIR  /var/lib/holzkube-manager-host
#   HOLZKUBE_MANAGER_SYSTEMCTL       systemctl
#
# HOLZKUBE_MANAGER_HOST_ORDER ist auch eine Schnittstelle fuer den Betrieb: ein
# Daemon mit anderem Datenverzeichnis braucht ein Drop-in, das sie setzt
# (deploy/HOST-HELPER.md, "A data directory other than
# /var/lib/holzkube-manager"). Die beiden anderen gibt es nur fuer die Tests in
# internal/host/hostaction und internal/httpapi, die eine Kopie dieses Skripts
# als root in einem User-Namespace laufen lassen und dabei weder die Pfade
# dieses Hosts noch sein echtes systemctl erreichen duerfen; im Betrieb setzt
# sie niemand.
ORDER=${HOLZKUBE_MANAGER_HOST_ORDER:-/var/lib/holzkube-manager/host-order}
STATE_DIR=${HOLZKUBE_MANAGER_HOST_STATE_DIR:-/var/lib/holzkube-manager-host}
SYSTEMCTL=${HOLZKUBE_MANAGER_SYSTEMCTL:-systemctl}

# Ein Auftrag, der aelter ist als MAX_AGE Sekunden, ist ein Rest -- etwa nach
# einem Stromausfall im Abholfenster --, kein Wunsch: PathExists= feuert auch
# beim Booten, und ein liegengebliebener "reboot" liefe sonst noch einmal. Einer
# aus der Zukunft ist ebenso verdaechtig; MAX_SKEW laesst Uhrzeitrauschen zu.
MAX_AGE=60
MAX_SKEW=5

log() { printf '%s\n' "$*"; }

# record id aktion ergebnis -- atomar, nur im eigenen Verzeichnis.
record() {
  local tmp
  tmp=$(mktemp "$STATE_DIR/.last.XXXXXX")
  printf '%s %s %s %s\n' "$1" "$2" "$3" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$tmp"
  chmod 0644 "$tmp"
  mv -f -- "$tmp" "$STATE_DIR/last"
}

# reject grund bytes [id aktion] -- nie den Inhalt protokollieren. id und
# aktion gibt es nur fuer einen Auftrag, der dem Muster entsprach; sonst steht
# "- -" in last, weil ungepruefter Text in einer Datei, die die Seite zeigt,
# genau die Einschleusung waere, die hier verhindert wird.
reject() {
  log "Auftrag verworfen: $1 (${2:-0} Byte)"
  record "${3:--}" "${4:--}" rejected
  exit 2
}

[[ $EUID -eq 0 ]] || { log "FEHLER: nur als root"; exit 1; }

# Das eigene Verzeichnis legt systemd an (StateDirectory=). Geprueft wird es
# trotzdem: root schreibt nur in ein echtes Verzeichnis, das root gehoert und
# in das niemand sonst schreiben kann -- sonst tauscht ein anderer die Datei
# von mktemp gegen einen Symlink, und root schriebe dorthin, wo er zeigt.
if [[ -L $STATE_DIR || ! -d $STATE_DIR ]]; then
  log "FEHLER: $STATE_DIR ist kein Verzeichnis"
  exit 1
fi
state_owner=$(stat -c %u -- "$STATE_DIR")
state_mode=$(stat -c %a -- "$STATE_DIR")
if [[ $state_owner != 0 ]] || (( 8#$state_mode & 8#022 )); then
  log "FEHLER: $STATE_DIR gehoert nicht root allein"
  exit 1
fi

if [[ ! -e $ORDER && ! -L $ORDER ]]; then
  log "kein Auftrag"
  exit 0
fi

# Ein Verzeichnis unter dem Namen des Auftrags legt nur ein kompromittierter
# Daemon an. root entfernt es nicht (kein rm -r im Verzeichnis eines anderen
# Benutzers) und handelt nicht.
if [[ -d $ORDER && ! -L $ORDER ]]; then
  log "FEHLER: Auftrag nicht entfernbar"
  record - - failed
  exit 1
fi

# Uebernehmen, bevor irgendetwas gelesen wird: ein rename(2) auf einen eigenen
# Namen im selben Verzeichnis. Der Daemon zieht einen Auftrag, den niemand
# abholt, genauso zurueck -- er benennt ihn um --, und rename ist ein einziger
# Systemaufruf: wer zuerst umbenennt, dem gehoert der Auftrag, und der andere
# findet den Namen leer. Ein "rm -f" nach dem Lesen saehe das nicht: es meldet
# auch dann Erfolg, wenn der Daemon den Auftrag schon zurueckgezogen hat, und
# es loeschte einen neueren Auftrag, der inzwischen unter dem Namen liegt.
# rename folgt keinem Symlink; die Vorsilbe .holzkube-manager-tmp- sieht die
# Path-Unit nicht, und der Daemon raeumt sie beim Start weg.
CLAIM="$(dirname -- "$ORDER")/.holzkube-manager-tmp-helper-claim-$$"
if ! mv -f -T -- "$ORDER" "$CLAIM" 2>/dev/null; then
  if [[ ! -e $ORDER && ! -L $ORDER ]]; then
    # Der Daemon hat ihn zurueckgezogen, bevor er hier ankam. Das ist kein
    # verworfener Auftrag, und "last" bleibt, wie es ist.
    log "Auftrag vor der Abholung zurueckgezogen"
    exit 0
  fi
  log "FEHLER: Auftrag nicht entfernbar"
  record - - failed
  exit 1
fi

# Ab hier gehoert der Auftrag diesem Lauf. Die Art nur fuer den
# protokollierten Grund; entschieden wird beim Lesen.
kind=regular
if [[ -L $CLAIM ]]; then
  kind=symlink
elif [[ ! -f $CLAIM ]]; then
  kind=not-regular
fi
mtime=$(stat -c %Y -- "$CLAIM" 2>/dev/null || echo 0)

work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT

# Das einzige Lesen: O_NOFOLLOW gegen einen untergeschobenen Symlink,
# O_NONBLOCK gegen ein FIFO, hoechstens 65 Byte -- eins mehr als ein Auftrag
# lang sein darf, damit ein zu langer als zu lang erkannt wird. Scheitert es,
# fehlt $work/o, und der Auftrag wird unten verworfen.
dd if="$CLAIM" of="$work/o" iflag=nofollow,nonblock bs=65 count=1 status=none 2>/dev/null || :

# Verbrauchen, bevor irgendetwas geschieht: der Name des Auftrags ist schon
# frei, jetzt geht auch die Datei. Ein Reboot findet danach nichts mehr.
if ! rm -f -- "$CLAIM"; then
  log "FEHLER: Auftrag nicht entfernbar"
  record - - failed
  exit 1
fi

[[ $kind == regular ]] || reject "$kind"
[[ -f $work/o ]] || reject "nicht lesbar"
size=$(stat -c %s -- "$work/o")
# Gelesen wurden hoechstens 65 Byte; wie lang die Datei wirklich war, weiss
# das Skript nicht, und das Journal soll keine erfundene Zahl tragen.
(( size <= 64 )) || reject "zu lang" "mehr als 64"
line=""
IFS= read -r line < "$work/o" || reject "keine vollstaendige Zeile" "$size"
# Genau eine Zeile und ihr Zeilenende, nichts davor, nichts danach.
(( size == ${#line} + 1 )) || reject "nicht genau eine Zeile" "$size"
[[ $line =~ ^(reboot|poweroff|restart-service|update|check-update)\ ([0-9a-f]{16})$ ]] || reject "unbekannte Form" "$size"
action=${BASH_REMATCH[1]}
id=${BASH_REMATCH[2]}

now=$(date +%s)
(( mtime >= now - MAX_AGE )) || reject "veraltet" "$size" "$id" "$action"
(( mtime <= now + MAX_SKEW )) || reject "aus der Zukunft" "$size" "$id" "$action"

# Fest je Aktion. Aus dem Auftrag stammt nur die Wahl des Zweigs.
case $action in
  reboot)          cmd=(reboot) ;;
  poweroff)        cmd=(poweroff) ;;
  restart-service) cmd=(restart holzkube-manager.service) ;;
  update)          cmd=(start --no-block holzkube-manager-update.service) ;;
  # Ohne --no-block, anders als update: der Start eines oneshot endet, wenn
  # sein ExecStart endet, also ist das Ende dieses systemctl das Ende der
  # Suche. Scheitert sie (GitHub nicht erreichbar, das Update-Skript fehlt),
  # scheitert systemctl, und unten steht "failed" fuer genau diesen Auftrag,
  # statt dass die Seite bis "keine Antwort" wartet. Die Unit begrenzt sich
  # selbst (TimeoutStartSec=2min plus TimeoutStopSec=15s), unter der Grenze
  # dieses Dienstes (3min).
  check-update)    cmd=(start holzkube-manager-update-check.service) ;;
  *)               reject "unbekannte Aktion" "$size" ;;
esac

# Vor dem Handeln festhalten: ein Reboot oder Poweroff kann dieses Skript
# beenden, bevor es danach noch etwas schreibt.
record "$id" "$action" started
log "Auftrag $id: $action"
if ! "$SYSTEMCTL" "${cmd[@]}"; then
  log "FEHLER: Auftrag $id ($action) gescheitert"
  record "$id" "$action" failed
  exit 1
fi
