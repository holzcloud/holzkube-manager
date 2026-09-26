#!/usr/bin/env python3
"""Hold a published release against what an operator actually downloads.

goreleaser reporting success means it uploaded what it built. It does not mean
the release carries what deploy/holzkube-manager-update.sh needs, and the two
came apart once already: the script picked the first asset whose name ended in
`linux_<arch>.tar.gz`, and the release began carrying two of those the moment
holzkubectl got an archive of its own.

So this asks the API for the release that now exists and checks the things
the update path depends on. Every one of them can be false while goreleaser is
green, which is the only reason to check them at all:

  - both linux architectures are there. "x86 und arm" is the requirement; a
    release missing one is a release that installs on half the fleet and says
    nothing about the other half.
  - exactly one daemon archive per architecture, because the script's selector
    would otherwise be choosing between two and could not say why.
  - checksums.txt, without which the script downloads and installs unverified,
    warns once into a log nobody reads, and carries on.
  - not a draft and not a prerelease, because /releases/latest skips both and a
    release nothing can see is not a release. This is the failure that looks
    like success from every angle except the host that needed it.
  - the notes are the changelog entry and not goreleaser's commit list.

Usage: verify-release.py <repo> <tag>   (GITHUB_TOKEN in the environment)
"""

import json
import os
import re
import sys
import urllib.error
import urllib.request

# The architectures a release must carry. Named here rather than derived from
# the response, because deriving them from what arrived would make this agree
# with whatever it is handed.
REQUIRED_LINUX_ARCHES = ("amd64", "arm64")

DAEMON_PREFIX = "holzkube-manager_"


def fetch(repo: str, tag: str) -> dict:
    url = f"https://api.github.com/repos/{repo}/releases/tags/{tag}"
    req = urllib.request.Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "Authorization": f"Bearer {os.environ['GITHUB_TOKEN']}",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as err:
        # A 404 here is the draft case and worth naming as such: the tag exists,
        # the archives exist, and this endpoint cannot see the release because
        # /releases/tags does not serve drafts -- which is exactly what the
        # update script would experience.
        if err.code == 404:
            sys.exit(
                f"::error::no published release for {tag}. Either it is still a draft, "
                f"in which case /releases/latest cannot see it either, or the tag and the "
                f"release have come apart."
            )
        raise


def main() -> int:
    repo, tag = sys.argv[1], sys.argv[2]
    release = fetch(repo, tag)
    names = [a["name"] for a in release.get("assets", [])]

    problems = []

    if release.get("draft"):
        problems.append("the release is a draft; /releases/latest skips drafts")
    if release.get("prerelease"):
        problems.append(
            "the release is marked prerelease; /releases/latest skips prereleases, so "
            "every host running the update script would keep installing the previous build "
            "and report itself up to date"
        )

    for arch in REQUIRED_LINUX_ARCHES:
        suffix = f"_linux_{arch}.tar.gz"
        hits = [n for n in names if n.startswith(DAEMON_PREFIX) and n.endswith(suffix)]
        if not hits:
            problems.append(f"no {DAEMON_PREFIX}*{suffix}: nothing to install on linux/{arch}")
        elif len(hits) > 1:
            problems.append(
                f"{len(hits)} daemon archives for linux/{arch} ({', '.join(hits)}); "
                f"the update script's selector would be picking one of them blind"
            )

    if "checksums.txt" not in names:
        problems.append(
            "no checksums.txt; the update script would install an unverified archive "
            "after one warning line"
        )

    # The version in the asset names is the tag without its v, and a mismatch
    # means goreleaser released something other than what was asked for.
    version = re.sub(r"^v", "", tag)
    mismatched = [
        n for n in names if n.startswith(DAEMON_PREFIX) and f"_{version}_" not in n
    ]
    if mismatched:
        problems.append(
            f"daemon archives that do not carry version {version}: {', '.join(mismatched)}"
        )

    # The notes are the changelog entry (release-notes.py). A page without its
    # heading means goreleaser was not handed them and fell back to the commit
    # list -- the release exists and says nothing an operator can read.
    if f"## What's new in {tag}" not in (release.get("body") or ""):
        problems.append(
            f"the release notes do not carry the changelog entry for {tag}; "
            f"the page shows something other than .github/release-notes.py's output"
        )

    print(f"release {tag}: {len(names)} assets")
    for name in sorted(names):
        print(f"  {name}")

    if problems:
        for problem in problems:
            print(f"::error::{problem}")
        return 1

    print(f"::notice::{tag} carries linux/{' and linux/'.join(REQUIRED_LINUX_ARCHES)}, "
          f"checksums, and is visible to /releases/latest")
    return 0


if __name__ == "__main__":
    sys.exit(main())
