"""Write the Go Markdown golden files from the Python converter.

For every internal/markdown/testdata/*.md, run fizzy_skill.markdown.to_html
on its text and write the result next to it as .html. Files are read and
written as raw UTF-8 bytes so carriage returns reach the converter unchanged.

Run from the repo root: python3 scripts/gen_markdown_golden.py
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from fizzy_skill.markdown import to_html  # noqa: E402

TESTDATA = ROOT / "internal" / "markdown" / "testdata"


def main() -> int:
    sources = sorted(TESTDATA.glob("*.md"))
    if not sources:
        print(f"no .md files in {TESTDATA}", file=sys.stderr)
        return 1
    for src in sources:
        text = src.read_bytes().decode("utf-8")
        src.with_suffix(".html").write_bytes(to_html(text).encode("utf-8"))
        print(src.with_suffix(".html").relative_to(ROOT))
    return 0


if __name__ == "__main__":
    sys.exit(main())
