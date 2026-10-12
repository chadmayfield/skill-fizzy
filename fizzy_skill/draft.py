"""Card drafts: one Markdown file per card.

    ---
    board: Garden
    column: Maybe?          # optional; default Maybe? (triage)
    tags: build, buy        # optional
    golden: false           # optional
    ---
    # Card title

    ## Goal
    One line on what done looks like.

    ## Details
    ...

    ## Links & media
    - [Install guide](https://example.com/guide)
    ![Rear axle](rear-axle.jpg)

    ## Steps
    - [ ] Measure ride height
    - [x] Order the hinge

Everything except the title and the "## Steps" section becomes the card
description. Steps become Fizzy steps. Image lines are uploaded and embedded;
relative image paths resolve against the draft's folder, then the inbox.
After a successful apply, "card:" is written back into the front matter so a
draft is never created twice.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List, Optional, Tuple

from . import markdown

_FRONT = re.compile(r"\A---\s*\n(.*?)\n---\s*\n", re.S)
_STEP = re.compile(r"^\s*[-*+]\s+\[([ xX])\]\s+(.*\S)\s*$")
_LINK = re.compile(r"\[[^\]]*\]\((https?://[^)\s]+)\)|(?<![(\"])\b(https?://[^\s<>)]+[^\s<>).,;:!?])")
_IMAGE = re.compile(r"!\[[^\]]*\]\(([^)\s]+)\)")

STEPS_HEADINGS = ("steps", "tasks", "checklist")


class DraftError(Exception):
    pass


@dataclass
class Draft:
    path: Path
    title: str
    board: str
    column: str = "Maybe?"
    tags: List[str] = field(default_factory=list)
    golden: bool = False
    card: Optional[int] = None
    body_md: str = ""
    steps: List[Tuple[str, bool]] = field(default_factory=list)
    meta: Dict[str, str] = field(default_factory=dict)

    def links(self) -> List[str]:
        found = []
        for m in _LINK.finditer(self.body_md):
            url = m.group(1) or m.group(2)
            if url not in found:
                found.append(url)
        return found

    def images(self) -> List[str]:
        """Targets of image lines; only these become attachments."""
        return [m.group(2) for m in map(markdown.IMAGE_LINE.match, self.body_md.splitlines()) if m]

    def check_images(self, inbox: Path) -> None:
        """Raise if an image would not embed or its file is missing."""
        for line in self.body_md.splitlines():
            if _IMAGE.search(line) and not markdown.IMAGE_LINE.match(line):
                raise DraftError(f"image must be on a line by itself to embed: {line.strip()!r}")
        for img in self.images():
            self.resolve_image(img, inbox)

    def resolve_image(self, target: str, inbox: Path) -> Path:
        p = Path(target).expanduser()
        candidates = [p] if p.is_absolute() else [self.path.parent / p, inbox / p]
        for c in candidates:
            if c.is_file():
                return c
        raise DraftError(f"image not found: {target} (looked in {', '.join(str(c) for c in candidates)})")

    def description_html(self, sgids: Optional[Dict[str, str]] = None) -> str:
        out = markdown.to_html(self.body_md)
        for target in self.images():
            placeholder = f"<!--attach:{target}-->"
            if sgids and target in sgids:
                out = out.replace(placeholder, f'<action-text-attachment sgid="{sgids[target]}"></action-text-attachment>')
            else:
                out = out.replace(placeholder, f"<p>[image: {target}]</p>")
        return out


def _parse_front(text: str) -> Dict[str, str]:
    meta: Dict[str, str] = {}
    for raw in text.splitlines():
        # A comment is whitespace + "# " (YAML style), so "#tag" values survive.
        line = re.split(r"\s+#(?:\s|$)", raw, maxsplit=1)[0].rstrip()
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if ":" not in line:
            raise DraftError(f"front matter line has no ':': {raw!r}")
        k, v = line.split(":", 1)
        meta[k.strip().lower()] = v.strip().strip('"').strip("'")
    return meta


def _split_list(value: str) -> List[str]:
    value = value.strip().strip("[]")
    return [t.strip().lstrip("#").strip().strip('"').lower() for t in value.split(",") if t.strip()]


def parse(path: Path) -> Draft:
    text = path.read_text()
    m = _FRONT.match(text)
    if not m:
        raise DraftError(f"{path}: missing front matter (--- board: ... ---)")
    meta = _parse_front(m.group(1))
    rest = text[m.end():]

    if "board" not in meta or not meta["board"]:
        raise DraftError(f"{path}: front matter needs 'board:'")

    lines = rest.split("\n")
    title = None
    body: List[str] = []
    steps: List[Tuple[str, bool]] = []
    in_steps = False
    for line in lines:
        h = re.match(r"^(#{1,6})\s+(.*?)\s*$", line)
        if h and len(h.group(1)) == 1 and title is None:
            title = h.group(2)
            continue
        if h and len(h.group(1)) == 2:
            in_steps = h.group(2).strip().lower() in STEPS_HEADINGS
            if in_steps:
                continue
        if in_steps:
            s = _STEP.match(line)
            if s:
                steps.append((s.group(2), s.group(1).lower() == "x"))
            elif line.strip():
                raise DraftError(f"{path}: lines under '## Steps' must be '- [ ] task' (got {line.strip()!r})")
            continue
        body.append(line)

    if not title:
        raise DraftError(f"{path}: needs a '# Title' line after the front matter")

    card = meta.get("card")
    return Draft(
        path=path,
        title=title.strip(),
        board=meta["board"],
        column=meta.get("column") or "Maybe?",
        tags=_split_list(meta.get("tags", "")),
        golden=meta.get("golden", "false").lower() in ("true", "yes", "1"),
        card=int(card) if card and card.isdigit() else None,
        body_md="\n".join(body).strip(),
        steps=steps,
        meta=meta,
    )


def record_card(path: Path, number: int) -> None:
    """Write card: <number> into the draft's front matter."""
    text = path.read_text()
    m = _FRONT.match(text)
    if not m:
        raise DraftError(f"{path}: missing front matter")
    front = [ln for ln in m.group(1).splitlines() if not ln.strip().lower().startswith("card:")]
    front.append(f"card: {number}")
    path.write_text("---\n" + "\n".join(front) + "\n---\n" + text[m.end():])
