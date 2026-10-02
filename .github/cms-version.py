#!/usr/bin/env python3
"""Write a project's released version into the holzcloud.ch pages.

holzcloud.ch is website 1 of the holzcloud-CMS. Each project has one page
per language (de, fr, it, rm, en), and the version stands in a few known
places on it. A release workflow calls this script after its tag exists, so
the site never shows an older version than the one that was released. The
pages fetch nothing at render time; the number is written into them.

Why not one shared value: the CMS has snippets ([[snippet:key]]), but a
snippet renders as block content (<p>…</p>), so it cannot stand inside a
sentence such as "Alpha 0.2.2: …" without breaking the paragraph, and it is
not expanded inside a link address. Until the CMS can embed a value inline,
the version is rewritten in place - but only where it is expected:

  * the text of the "stand" blocks (the status boxes), where every bare
    X.Y.Z is the current version;
  * the project's own release links (…/releases/tag/vX.Y.Z and, for
    holzIce, the download link and the archive name) in "stand" and "text"
    blocks.

Nothing else on a page is touched: "New in 0.0.2" in a feature list is
history, and the alt text of a screenshot describes the version the
screenshot shows. Before anything is written, every page of the project is
read and every pattern is counted; if one count differs from what this file
expects, nothing is written at all. A changed page layout therefore stops
the update loudly instead of rewriting the wrong number.

Usage:
  CMS_TOKEN=… cms-version.py --project holzkube-manager --version v0.2.3
  cms-version.py --project holzice --version 0.0.6 --dry-run

The token is a holzcloud-CMS AI key of level "content", limited to website
1 (made in the CMS admin under "KI-Zugang"). Without CMS_TOKEN the script
prints a notice and exits 0, so a release keeps working until the secret
exists. Exit codes: 0 done or skipped, 1 refused or failed.

The endpoint is the CMS's MCP address: JSON-RPC 2.0, POST, bearer key,
stateless - one tools/call per request. Python standard library only.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

DEFAULT_ENDPOINT = "https://holzcloud.ch/ai"
DEFAULT_SITE = "https://holzcloud.ch"
WEBSITE = 1
MAIN_LANGUAGE = "de"

V = r"(\d+\.\d+\.\d+)"

# A bare version: not part of a longer number, a word, a path or a pre-release
# suffix ("0.0.6-beta1"). The trailing sentence dot of "… 0.2.2." is allowed.
BARE = r"(?<![\w./-])" + V + r"(?!\.?\d)(?![\w-])"


def tag_link(repo: str) -> str:
    """A release link; repo is a regex, so a renamed repository can be named too."""
    return r"github\.com/holzcloud/(?:" + repo + r")/releases/tag/v" + V + r"(?![\w.-])"


# Each rule: (where, regex with exactly one group around the version, count
# per page). "where" names the block fields the regex is applied to:
#   stand -> fields.text of a "stand" block
#   text  -> markdown of a "text" block
PROJECTS = {
    "holzkube-manager": {
        "slug": "holzkube-manager",
        "pages": 5,
        "rules": [
            ("stand", BARE, 2),
            ("stand text", tag_link("holzkube-manager"), 2),
        ],
    },
    "holzcloud-cms": {
        "slug": "holzcloud-cms",
        "pages": 5,
        "rules": [
            ("stand", BARE, 2),
            ("stand text", tag_link("holzcloud-cms"), 1),
        ],
    },
    "hauscloud": {
        "slug": "hauscloud",
        "pages": 5,
        "rules": [
            ("stand", BARE, 2),
        ],
    },
    # The repository was renamed from holzIce to holzBar on 2026-10-02; the
    # pages may carry either name while they catch up, so both are accepted.
    "holzice": {
        "slug": ("holzice", "holzbar"),
        "pages": 5,
        "rules": [
            ("stand", BARE, 1),
            ("stand text", tag_link("holzIce|holzBar"), 1),
            ("text", r"github\.com/holzcloud/(?:holzIce|holzBar)/releases/download/v" + V + r"/", 1),
            ("text", r"(?:holzIce|holzBar)-" + V + r"\.zip", 2),
        ],
    },
}
# The names the release workflows use, so a caller can pass its repository.
ALIASES = {"hauscloud.ch": "hauscloud", "holzIce": "holzice", "holzBar": "holzice", "holzbar": "holzice"}


class Refused(Exception):
    """Something is not as expected; nothing must be written."""


# ---------------------------------------------------------------- the CMS


class CMS:
    def __init__(self, endpoint: str, token: str, timeout: float = 30.0):
        self.endpoint = endpoint
        self.token = token
        self.timeout = timeout
        self._id = 0

    def call(self, tool: str, arguments: dict):
        """One tools/call. Returns the decoded JSON the tool answered."""
        self._id += 1
        body = json.dumps({
            "jsonrpc": "2.0", "id": self._id, "method": "tools/call",
            "params": {"name": tool, "arguments": arguments},
        }).encode()
        req = urllib.request.Request(self.endpoint, data=body, method="POST", headers={
            "Authorization": "Bearer " + self.token,
            "Content-Type": "application/json",
            "Accept": "application/json",
            "User-Agent": "holzcloud-cms-version/1",
        })
        last = None
        for attempt in range(3):
            try:
                with urllib.request.urlopen(req, timeout=self.timeout) as res:
                    answer = json.load(res)
                break
            except urllib.error.HTTPError as e:
                if e.code in (401, 403):
                    raise Refused(f"the CMS refused the key (HTTP {e.code}); "
                                  "is CMS_TOKEN a valid content key for website 1?")
                last = e
                if e.code < 500:
                    raise Refused(f"the CMS answered HTTP {e.code} to {tool}")
            except (urllib.error.URLError, TimeoutError) as e:
                last = e
            time.sleep(2 * (attempt + 1))
        else:
            raise Refused(f"the CMS could not be reached for {tool}: {last}")

        if answer.get("error"):
            raise Refused(f"{tool}: {answer['error'].get('message')}")
        result = answer.get("result") or {}
        text = "".join(c.get("text", "") for c in result.get("content", []))
        if result.get("isError"):
            raise ToolError(tool, text)
        try:
            return json.loads(text)
        except json.JSONDecodeError:
            raise Refused(f"{tool} answered something that is not JSON: {text[:200]}")


class ToolError(Refused):
    """The tool answered with an error (isError) rather than a result."""

    def __init__(self, tool: str, message: str):
        super().__init__(f"{tool}: {message}")
        self.message = message


def is_conflict(err: ToolError) -> bool:
    return "saved the page in the meantime" in err.message


# ---------------------------------------------------------- the rewriting


def fields_of(block: dict, where: str):
    """Yield (container, key) for every string a rule applies to."""
    kinds = where.split()
    if "stand" in kinds and block.get("type") == "stand":
        f = block.get("fields") or {}
        if isinstance(f.get("text"), str):
            yield f, "text"
    if "text" in kinds and block.get("type") == "text":
        if isinstance(block.get("markdown"), str):
            yield block, "markdown"


def find(blocks: list, rules: list) -> list:
    """Every version a rule finds, as [(rule index, version)]."""
    out = []
    for i, (where, rx, _) in enumerate(rules):
        pattern = re.compile(rx)
        for block in blocks:
            for container, key in fields_of(block, where):
                for m in pattern.finditer(container[key]):
                    out.append((i, m.group(1)))
    return out


def rewrite(blocks: list, rules: list, version: str) -> tuple[list, list]:
    """A copy of blocks with every match set to version, and the changes."""
    blocks = json.loads(json.dumps(blocks))
    originals = {}
    for where, rx, _ in rules:
        pattern = re.compile(rx)

        def put(m: re.Match) -> str:
            whole = m.group(0)
            s, e = m.start(1) - m.start(0), m.end(1) - m.start(0)
            return whole[:s] + version + whole[e:]

        for block in blocks:
            for container, key in fields_of(block, where):
                before = container[key]
                after = pattern.sub(put, before)
                if after != before:
                    originals.setdefault((id(container), key), (block.get("type"), container, before))
                    container[key] = after
    changes = [(kind, before, c[key]) for (_, key), (kind, c, before) in originals.items()]
    return blocks, changes


def check_counts(page: dict, blocks: list, rules: list) -> list:
    """The versions found on one page, after checking every count."""
    found = find(blocks, rules)
    wrong = []
    for i, (where, rx, expected) in enumerate(rules):
        n = sum(1 for r, _ in found if r == i)
        if n != expected:
            wrong.append(f"rule {i + 1} ({where}: /{rx}/) found {n}, expects {expected}")
    if wrong:
        raise Refused(f"page {page['id']} ({label(page)}): " + "; ".join(wrong))
    return [v for _, v in found]


def vkey(v: str) -> tuple:
    return tuple(int(x) for x in v.split("."))


def label(page: dict) -> str:
    return f"{page.get('language') or MAIN_LANGUAGE}/{page['slug']}"


def public_path(page: dict) -> str:
    lang = page.get("language") or MAIN_LANGUAGE
    return "/" + page["slug"] if lang == MAIN_LANGUAGE else f"/{lang}/{page['slug']}"


# ------------------------------------------------------------------ the run


def notice(msg: str):
    print(f"::notice::{msg}" if os.environ.get("GITHUB_ACTIONS") else f"notice: {msg}")


def warn(msg: str):
    print(f"::warning::{msg}" if os.environ.get("GITHUB_ACTIONS") else f"warning: {msg}")


def project_pages(cms: CMS, slug, expected: int) -> list:
    slugs = (slug,) if isinstance(slug, str) else tuple(slug)
    listed = cms.call("list_pages", {"website": WEBSITE, "status": "all", "limit": 500})
    pages = [p for p in listed.get("pages", []) if p.get("slug") in slugs]
    if len(pages) != expected:
        raise Refused(f"website {WEBSITE} has {len(pages)} pages with the address "
                      f"{' or '.join(slugs)}, expected {expected} (one per language)")
    return pages


def run(args) -> int:
    project = ALIASES.get(args.project, args.project)
    conf = PROJECTS[project]
    rules = conf["rules"]

    version = args.version.strip()
    version = version[1:] if version.startswith("v") else version
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        notice(f"'{args.version}' is not a plain X.Y.Z release (a beta or a "
               "release candidate); holzcloud.ch keeps showing the last plain release.")
        return 0

    token = os.environ.get("CMS_TOKEN", "").strip()
    if not token:
        notice("CMS_TOKEN is not set, so holzcloud.ch was not updated. Add the "
               "secret CMS_TOKEN (a content key for website 1) to update it on "
               "every release.")
        return 0

    cms = CMS(args.endpoint, token)
    pages = project_pages(cms, conf["slug"], conf["pages"])

    # Read and check every page before writing any: either all pages are
    # updated or none is.
    plan = []
    for page in pages:
        got = cms.call("get_page_blocks", {"id": page["id"]})
        current = check_counts(page, got["blocks"], rules)
        plan.append((page, got, current))

    newest = max((v for _, _, cur in plan for v in cur), key=vkey, default=None)
    if newest and vkey(newest) > vkey(version) and not args.allow_older:
        notice(f"holzcloud.ch already shows {newest}, newer than {version}; "
               "nothing written (--allow-older overrides).")
        return 0

    todo = []
    for page, got, current in plan:
        blocks, changes = rewrite(got["blocks"], rules, version)
        print(f"{label(page)} (page {page['id']}, version {got['version']}): "
              f"found {', '.join(current) or 'nothing'}")
        for kind, before, after in changes:
            for a, b in zip(before.splitlines(), after.splitlines()):
                if a != b:
                    print(f"  [{kind}] - {a.strip()[:160]}")
                    print(f"  [{kind}] + {b.strip()[:160]}")
        if changes:
            todo.append((page, got))

    if not todo:
        print(f"All {len(plan)} pages of {project} already show {version}.")
        return 0
    if args.dry_run:
        print(f"Dry run: {len(todo)} of {len(plan)} pages would be written.")
        return 0

    for page, got in todo:
        write(cms, page, got, rules, version)

    # Read back. A write the CMS accepted is not yet a page that says the
    # right thing; this is the check that it does.
    for page in pages:
        got = cms.call("get_page_blocks", {"id": page["id"]})
        current = check_counts(page, got["blocks"], rules)
        stale = sorted({v for v in current if v != version})
        if stale:
            raise Refused(f"page {page['id']} ({label(page)}) still shows {', '.join(stale)} after the write")
    print(f"Read back: all {len(pages)} pages of {project} show {version}.")

    if args.site:
        check_public(args.site, pages, version)
    return 0


def write(cms: CMS, page: dict, got: dict, rules: list, version: str):
    for attempt in range(4):
        blocks, _ = rewrite(got["blocks"], rules, version)
        try:
            res = cms.call("set_page_blocks", {
                "id": page["id"], "blocks": blocks, "version": got["version"]})
            print(f"wrote {label(page)}: version {got['version']} -> {res.get('version')}")
            return
        except ToolError as e:
            if not is_conflict(e) or attempt == 3:
                raise Refused(str(e))
            # Somebody saved in between: read again, check again, retry.
            notice(f"{label(page)} changed while it was being updated; reading it again")
            time.sleep(1 + attempt)
            got = cms.call("get_page_blocks", {"id": page["id"]})
            check_counts(page, got["blocks"], rules)


def check_public(site: str, pages: list, version: str):
    """What a visitor gets. A warning only: a cache may lag behind."""
    for page in pages:
        url = site.rstrip("/") + public_path(page)
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "holzcloud-cms-version/1"})
            with urllib.request.urlopen(req, timeout=30) as res:
                html = res.read().decode("utf-8", "replace")
        except Exception as e:  # noqa: BLE001 - reported, not fatal
            warn(f"{url} could not be fetched: {e}")
            continue
        if version in html:
            print(f"public {url}: shows {version}")
        else:
            warn(f"{url} does not show {version} yet")


def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    p.add_argument("--project", required=True, choices=sorted(set(PROJECTS) | set(ALIASES)))
    p.add_argument("--version", required=True, help="the released tag, e.g. v0.2.3")
    p.add_argument("--dry-run", action="store_true", help="read and print the plan, write nothing")
    p.add_argument("--allow-older", action="store_true", help="write even if the site shows a newer version")
    p.add_argument("--endpoint", default=os.environ.get("CMS_ENDPOINT") or DEFAULT_ENDPOINT)
    p.add_argument("--site", default=os.environ.get("CMS_SITE", DEFAULT_SITE),
                   help="public base address to check after the write; empty skips it")
    args = p.parse_args(argv)
    try:
        return run(args)
    except Refused as e:
        msg = f"holzcloud.ch was not updated: {e}"
        print(f"::error::{msg}" if os.environ.get("GITHUB_ACTIONS") else f"error: {msg}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
