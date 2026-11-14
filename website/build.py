#!/usr/bin/env python3
"""Assemble the documentation website from the repository.

The Markdown in docs/, the example READMEs and the project files at the
repository root are the single source; this script copies them into
website/.build/docs, rewrites their relative links for the site layout, and
leaves the rest to MkDocs:

    python3 website/build.py && mkdocs build --strict -f website/mkdocs.yml

Links that point at files the site does not contain (source code, JSON
Schemas, example configuration) become links to the repository on GitHub.
"""

import posixpath
import re
import shutil
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "website" / ".build" / "docs"
REPO = "https://github.com/BhuwanJoshi-01/prodshadow"

# Repository files (paths relative to the root) that become site pages, by
# their path in the site.
ROOT_PAGES = {
    "README.md": "introduction.md",
    "CHANGELOG.md": "changelog.md",
    "CONTRIBUTING.md": "contributing.md",
    "SECURITY.md": "security.md",
    "CODE_OF_CONDUCT.md": "code-of-conduct.md",
}

LINK = re.compile(r"(?<!!)\[([^\]]*)\]\(([^)\s]+)((?:\s+\"[^\"]*\")?)\)")


def site_path(repo_path: str) -> str | None:
    """Return the site page for a repository path, or None if the site does
    not contain it."""
    p = repo_path.rstrip("/")
    if p in ROOT_PAGES:
        return ROOT_PAGES[p]
    if p == "docs" or p == "docs/README.md":
        return "index.md"
    if p.startswith("docs/"):
        rel = p[len("docs/"):]
        if rel.endswith("README.md"):
            return rel[: -len("README.md")] + "index.md"
        if rel.endswith(".md"):
            return rel
        if (ROOT / p).is_dir() and (ROOT / p / "README.md").exists():
            return rel + "/index.md"
        return None
    if p in ("examples", "examples/README.md"):
        return "examples/index.md"
    m = re.fullmatch(r"examples/([^/]+)(?:/README\.md)?", p)
    if m and (ROOT / "examples" / m.group(1) / "README.md").exists():
        return f"examples/{m.group(1)}.md"
    return None


def rewrite(text: str, source: str, dest: str) -> str:
    """Rewrite the relative links of a page copied from repository path
    source to site path dest."""

    def fix(m: re.Match) -> str:
        label, target, title = m.group(1), m.group(2), m.group(3)
        if re.match(r"^[a-z][a-z0-9+.-]*:", target) or target.startswith("#"):
            return m.group(0)
        path, _, anchor = target.partition("#")
        resolved = posixpath.normpath(posixpath.join(posixpath.dirname(source), path))
        if resolved.startswith(".."):
            return m.group(0)
        page = site_path(resolved)
        if page is None:
            kind = "tree" if (ROOT / resolved).is_dir() else "blob"
            new = f"{REPO}/{kind}/main/{resolved}"
        else:
            new = posixpath.relpath(page, posixpath.dirname(dest) or ".")
        if anchor:
            new += "#" + anchor
        return f"[{label}]({new}{title})"

    return LINK.sub(fix, text)


def copy(source: str, dest: str) -> None:
    text = (ROOT / source).read_text(encoding="utf-8")
    out = OUT / dest
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(rewrite(text, source, dest), encoding="utf-8")


def main() -> int:
    if OUT.exists():
        shutil.rmtree(OUT)
    OUT.mkdir(parents=True)

    for md in sorted((ROOT / "docs").rglob("*.md")):
        source = md.relative_to(ROOT).as_posix()
        copy(source, site_path(source))
    for source, dest in ROOT_PAGES.items():
        if (ROOT / source).exists():
            copy(source, dest)
    copy("examples/README.md", "examples/index.md")
    for readme in sorted((ROOT / "examples").glob("*/README.md")):
        source = readme.relative_to(ROOT).as_posix()
        copy(source, site_path(source))

    # The home page and the site's own assets live in website/.
    shutil.copy(ROOT / "website" / "home.md", OUT / "index.md")
    shutil.copytree(ROOT / "website" / "assets", OUT / "assets")
    print(f"assembled {sum(1 for _ in OUT.rglob('*.md'))} pages in {OUT.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
