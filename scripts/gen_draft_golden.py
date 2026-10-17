"""Write the Go draft golden files from the Python draft parser.

For every internal/draft/testdata/*.md, run fizzy_skill.draft.parse and write
the result next to it as .json (title, board, column, tags, golden, card,
body_md, steps, links, images, meta). For every testdata/errors/*.md, write a
.json holding {"error": true} when parse raises DraftError.

Run from the repo root: python3 scripts/gen_draft_golden.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from fizzy_skill import draft  # noqa: E402

TESTDATA = ROOT / "internal" / "draft" / "testdata"


def dump(path: Path) -> dict:
    d = draft.parse(path)
    return {
        "title": d.title,
        "board": d.board,
        "column": d.column,
        "tags": d.tags,
        "golden": d.golden,
        "card": d.card,
        "body_md": d.body_md,
        "steps": [{"content": c, "done": done} for c, done in d.steps],
        "links": d.links(),
        "images": d.images(),
        "meta": d.meta,
    }


def write(path: Path, data: dict) -> None:
    path.with_suffix(".json").write_text(
        json.dumps(data, indent=2, ensure_ascii=False, sort_keys=True) + "\n", encoding="utf-8"
    )


def main() -> int:
    sources = sorted(TESTDATA.glob("*.md"))
    errors = sorted((TESTDATA / "errors").glob("*.md"))
    if not sources or not errors:
        print(f"missing fixtures in {TESTDATA}", file=sys.stderr)
        return 1
    for src in sources:
        write(src, dump(src))
    for src in errors:
        try:
            draft.parse(src)
        except draft.DraftError:
            write(src, {"error": True})
        else:
            print(f"{src.name}: expected DraftError", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
