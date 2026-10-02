#!/usr/bin/env bash
# The brand tokens live in holzcloud-design. This application needs shadcn's
# own variable names, and it has no package manager for CSS, so the values sit
# in web/src/index.css as a copy.
#
# A copy drifts. This script fetches tokens.css from the pinned tag and compares
# every --hc- declaration that appears in both files. It does not require that
# all tokens are copied - only that none of them means something different here
# than it does upstream.
#
# The accent is the exception, and it is compared against the right file rather
# than excused. holzkube's accent is Glut, and holzcloud-design carries Glut in
# the holzkube scope of css/programs.css ([data-hc-program="holzkube"]), while
# tokens.css keeps brass, which is holzcloud's. So the five accent tokens below
# are compared with that scope, a var() there resolved through tokens.css, and
# everything else with tokens.css as before.
#
# Comparison is on VALUES, not on text. Biome formats this repository's CSS and
# normalises what the template writes by hand: #0A0705 becomes #0a0705, and .55
# becomes 0.55. A textual diff would fail on every run and teach everyone to
# ignore it, which is worse than no check.
set -euo pipefail

TAG="$(cat "$(dirname "$0")/../../.design-version")"
BASE="https://raw.githubusercontent.com/holzcloud/holzcloud-design/${TAG}/css"
LOCAL="$(dirname "$0")/../../web/src/index.css"

echo "holzcloud-design ${TAG}"
curl -fsSL "$BASE/tokens.css" -o /tmp/hc-tokens.css
curl -fsSL "$BASE/programs.css" -o /tmp/hc-programs.css

python3 - "$TAG" /tmp/hc-tokens.css /tmp/hc-programs.css "$LOCAL" <<'PY'
import re, sys

tag, upstream_path, programs_path, local_path = sys.argv[1:5]

# The accent tokens this application sets to Glut. Upstream they are defined
# per program in css/programs.css, not in tokens.css.
ACCENT = ['--hc-brass', '--hc-brass-soft', '--hc-brass-wash', '--hc-wash-brass', '--hc-pane-lift']
PROGRAM = 'holzkube'

DECL = re.compile(r'(--hc-[a-z0-9-]+)\s*:\s*([^;]+);', re.S)
COMMENT = re.compile(r'/\*.*?\*/', re.S)

def norm(v: str) -> str:
    """Compare what a value MEANS, not how it is typed."""
    v = ' '.join(v.split())                                  # collapse whitespace
    v = v.lower()                                            # #0A0705 == #0a0705
    v = re.sub(r'(?<![\w.])\.(\d)', r'0.\1', v)              # .55 == 0.55
    v = re.sub(r'\s*,\s*', ',', v)                           # spacing around commas
    v = re.sub(r'\s*/\s*', '/', v)                           # rgb(a b c / d)
    # 0.40 == 0.4: biome strips the trailing zero the template writes.
    # Only inside a decimal, and only trailing zeros. The rule that also
    # stripped a bare trailing dot had to go: it matched the dot in
    # "www.w3.org" inside the cube mask's data URI and reported a drift it
    # had created itself - w3.org became w3org on one side only.
    v = re.sub(r'(\d+\.\d*?)0+\b', r'\1', v)
    v = re.sub(r'(\d+)\.(?=\s|,|\)|$)', r'\1', v)
    return v

def read(path):
    # Comments first, and not as a nicety: a prose comment in this file
    # mentions "--hc-on-brass: 9.95:1" while explaining the ratio, and a
    # parser that reads comments takes that as the declaration. The check
    # then reports a drift it invented itself.
    text = COMMENT.sub(' ', open(path).read())
    return {m.group(1): norm(m.group(2)) for m in DECL.finditer(text)}

def program_scope(path, program):
    # The block that opens with :root[data-hc-program="<program>"]. Rules in
    # programs.css hold declarations only, so the first closing brace ends it.
    text = COMMENT.sub(' ', open(path).read())
    m = re.search(r'\[data-hc-program="' + re.escape(program) + r'"\][^{]*\{([^}]*)\}', text)
    if not m:
        print(f'  FEHLER: programs.css hat keinen Bereich fuer {program}'); sys.exit(1)
    return {d.group(1): d.group(2) for d in DECL.finditer(m.group(1))}

up = read(upstream_path)
raw_tokens = {m.group(1): m.group(2) for m in DECL.finditer(COMMENT.sub(' ', open(upstream_path).read()))}
scope = program_scope(programs_path, PROGRAM)
for k in ACCENT:
    if k not in scope:
        print(f'  FEHLER: {k} fehlt im Bereich {PROGRAM} von programs.css'); sys.exit(1)
    v = scope[k].strip()
    ref = re.fullmatch(r'var\(\s*(--hc-[a-z0-9-]+)\s*\)', v)
    if ref:
        v = raw_tokens[ref.group(1)]                         # --hc-family-holzkube
    up[k] = norm(v)
print(f'  {len(ACCENT)} Akzent-Tokens aus programs.css [data-hc-program="{PROGRAM}"]')
lo = read(local_path)

print(f'  {len(up)} Tokens im Original, {len(lo)} hier')

shared = sorted(set(up) & set(lo))
if not shared:
    print('  FEHLER: kein einziges Token kommt in beiden Dateien vor'); sys.exit(1)

bad = 0
for k in shared:
    if up[k] != lo[k]:
        print(f'  ABWEICHUNG {k}')
        print(f'    Original: {up[k]}')
        print(f'    hier    : {lo[k]}')
        bad += 1

print(f'  {len(shared)} gemeinsame Werte geprueft')
if bad:
    print(f'  {bad} Abweichung(en) zu holzcloud-design {tag}'); sys.exit(1)
print(f'  alle Werte stimmen mit holzcloud-design {tag} ueberein')
PY
