#!/usr/bin/env python3
"""Write a release's notes from its changelog entry.

Every release carries its changelog on the release page (the operator's rule of
2026-09-26). Until then the page showed goreleaser's commit list -- v1.32.0's
opened on three "Merge branch 'worktree-agent-…'" lines -- while the words
written for the operator sat in internal/changelog/changelog.json and were only
ever seen inside the app. This makes the page say the same thing the app's
"What's new" panel says, from the same file.

The release job has already refused a tag without an entry, so a missing one
here is a second, cheaper refusal rather than a fallback: notes generated from
the commit log are the thing the changelog exists not to be.

Usage: release-notes.py <tag> [annotation-file] > notes.md
"""

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent


def heading(tag: str) -> str:
    # verify-release.py looks for exactly this line on the published release.
    return f"## What's new in {tag}"


def render(tag: str, annotation: str = "") -> str:
    releases = json.loads((ROOT / "internal/changelog/changelog.json").read_text())
    entry = next((r for r in releases if r["version"] == tag), None)
    if entry is None:
        sys.exit(f"::error::internal/changelog/changelog.json has no entry for {tag}")
    if not entry.get("changes"):
        sys.exit(f"::error::the changelog entry for {tag} lists no changes")

    lines = [heading(tag), "", f"_{entry['date']}_", ""]
    for change in entry["changes"]:
        lines.append(f"- {change.get('icon', '•')} {change['text']}")
    annotation = annotation.strip()
    # The dispatch input is optional and defaults to the tag name itself, which
    # says nothing the heading does not.
    if annotation and annotation != tag:
        lines += ["", annotation]
    lines += [
        "",
        "---",
        "",
        "**Install or update:** download the archive for your machine below — "
        "`linux_arm64` for a Raspberry Pi, `linux_amd64` otherwise — or run "
        "`sudo holzkube-manager-update` on a host that already has it. "
        "The [guide](https://github.com/holzcloud/holzkube-manager/blob/main/docs/guide.md) "
        "has the rest.",
    ]
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    tag = sys.argv[1]
    annotation = pathlib.Path(sys.argv[2]).read_text() if len(sys.argv) > 2 else ""
    sys.stdout.write(render(tag, annotation))
