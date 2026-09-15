#!/usr/bin/env python3
"""Is there a newer stable Talos machinery than the one this repository pins?

The operator updates Talos as soon as a release lands, so a pin that lags is not
a tidiness question — it is the product advertising support for a cluster whose
configuration its own library cannot read. That shipped once: machinery v1.13.9
against a v1.14 cluster, and adoption failed on a document kind the older
machinery had never heard of.

Nothing in a build can notice this, because nothing in a build looks outside.
So it is asked on a schedule instead, and a newer release makes the run fail —
which is the notification. A check that only reported into a log nobody reads
would repeat ledger entries 5 and 64, where an opt-in drift watcher existed and
was never scheduled.

Pre-releases are ignored on purpose: alpha and rc are not what an operator
installs, and failing on them would teach everybody to ignore this.
"""

import json
import re
import sys
import urllib.request

MODULE = "github.com/siderolabs/talos/pkg/machinery"
PROXY = f"https://proxy.golang.org/{MODULE}/@v/list"

STABLE = re.compile(r"^v(\d+)\.(\d+)\.(\d+)$")


def pinned() -> str:
    with open("go.mod", encoding="utf-8") as f:
        for line in f:
            if MODULE in line:
                return line.split()[-1]
    sys.exit(f"::error::{MODULE} is not in go.mod at all")


def stable_versions() -> list[tuple[int, int, int]]:
    with urllib.request.urlopen(PROXY, timeout=60) as resp:
        body = resp.read().decode()
    out = []
    for line in body.splitlines():
        m = STABLE.match(line.strip())
        if m:
            out.append(tuple(int(g) for g in m.groups()))
    return sorted(out)


def main() -> int:
    have = pinned()
    m = STABLE.match(have)
    if m is None:
        print(f"::notice::the pin is {have}, which is not a stable release; not comparing")
        return 0
    current = tuple(int(g) for g in m.groups())

    versions = stable_versions()
    if not versions:
        sys.exit("::error::the proxy listed no stable versions; the check could not be made")
    newest = versions[-1]

    have_s = "v%d.%d.%d" % current
    newest_s = "v%d.%d.%d" % newest

    if newest <= current:
        print(f"::notice::machinery {have_s} is the newest stable release")
        return 0

    print(
        f"::error::machinery {newest_s} is out and this repository pins {have_s}. "
        f"The operator updates Talos as soon as it lands, so the pin lagging means this "
        f"product may advertise a Talos whose configuration documents its own library "
        f"cannot decode. Raise the pin in go.mod, run the gate, and move "
        f"MaxSupportedVersion with it if the minor changed."
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
