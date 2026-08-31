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
# Comparison is on VALUES, not on text. Biome formats this repository's CSS and
# normalises what the template writes by hand: #0A0705 becomes #0a0705, and .55
# becomes 0.55. A textual diff would fail on every run and teach everyone to
# ignore it, which is worse than no check.
set -euo pipefail

TAG="$(cat "$(dirname "$0")/../../.design-version")"
URL="https://raw.githubusercontent.com/holzcloud/holzcloud-design/${TAG}/css/tokens.css"
LOCAL="$(dirname "$0")/../../web/src/index.css"

echo "holzcloud-design ${TAG}"
curl -fsSL "$URL" -o /tmp/hc-tokens.css

python3 - "$TAG" /tmp/hc-tokens.css "$LOCAL" <<'PY'
import re, sys

tag, upstream_path, local_path = sys.argv[1], sys.argv[2], sys.argv[3]

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
    v = re.sub(r'(\d+\.\d*?)0+(?=\D|$)', r'\1', v)
    v = re.sub(r'(\d+)\.(?=\D|$)', r'\1', v)
    return v

def read(path):
    # Comments first, and not as a nicety: a prose comment in this file
    # mentions "--hc-on-brass: 9.95:1" while explaining the ratio, and a
    # parser that reads comments takes that as the declaration. The check
    # then reports a drift it invented itself.
    text = COMMENT.sub(' ', open(path).read())
    return {m.group(1): norm(m.group(2)) for m in DECL.finditer(text)}

up = read(upstream_path)
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
