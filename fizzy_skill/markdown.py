"""Minimal Markdown -> HTML for card descriptions (stdlib only).

Supports what card drafts need and nothing more: ATX headings, paragraphs,
bullet and numbered lists (one level), task-list items, fenced code blocks,
blockquotes, links, bold, italic, inline code, horizontal rules, and image
lines. Fizzy sanitizes rich text on its side as well.

Images: a line that is only ![alt](target) becomes an attachment placeholder
<!--attach:target--> which the draft layer replaces with an uploaded file.
"""

from __future__ import annotations

import html
import re
from typing import List

_IMAGE_LINE = re.compile(r"^\s*!\[([^\]]*)\]\(([^)\s]+)\)\s*$")
_HEADING = re.compile(r"^(#{1,6})\s+(.*?)\s*#*\s*$")
_BULLET = re.compile(r"^\s*[-*+]\s+(.*)$")
_NUMBERED = re.compile(r"^\s*\d+[.)]\s+(.*)$")
_TASK = re.compile(r"^\[([ xX])\]\s+(.*)$")
_HR = re.compile(r"^\s*([-*_])(\s*\1){2,}\s*$")


def inline(text: str) -> str:
    """Escape, then apply inline code, links, bold and italic."""
    codes: List[str] = []

    def stash(m: "re.Match[str]") -> str:
        codes.append(f"<code>{html.escape(m.group(1))}</code>")
        return f"\x00{len(codes) - 1}\x00"

    text = re.sub(r"`([^`]+)`", stash, text)
    text = html.escape(text, quote=False)
    text = re.sub(r"\[([^\]]+)\]\((https?://[^)\s]+|mailto:[^)\s]+)\)",
                  lambda m: f'<a href="{html.escape(m.group(2))}">{m.group(1)}</a>', text)
    text = re.sub(r"(?<![\"=>])\b(https?://[^\s<]+[^\s<.,;:!?)])", r'<a href="\1">\1</a>', text)
    text = re.sub(r"\*\*(.+?)\*\*", r"<strong>\1</strong>", text)
    text = re.sub(r"(?<![*\w])\*(?!\s)(.+?)(?<!\s)\*(?![*\w])", r"<em>\1</em>", text)
    text = re.sub(r"(?<!\w)_(?!\s)(.+?)(?<!\s)_(?!\w)", r"<em>\1</em>", text)
    return re.sub(r"\x00(\d+)\x00", lambda m: codes[int(m.group(1))], text)


def to_html(md: str) -> str:
    lines = md.replace("\r\n", "\n").split("\n")
    out: List[str] = []
    para: List[str] = []
    list_tag = None  # "ul" | "ol"
    i = 0

    def flush_para() -> None:
        if para:
            out.append("<p>" + "<br>".join(inline(p) for p in para) + "</p>")
            para.clear()

    def close_list() -> None:
        nonlocal list_tag
        if list_tag:
            out.append(f"</{list_tag}>")
            list_tag = None

    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        if stripped.startswith("```"):
            flush_para(); close_list()
            code: List[str] = []
            i += 1
            while i < len(lines) and not lines[i].strip().startswith("```"):
                code.append(lines[i])
                i += 1
            out.append("<pre>" + html.escape("\n".join(code)) + "</pre>")
            i += 1
            continue

        if not stripped:
            flush_para(); close_list()
            i += 1
            continue

        m = _IMAGE_LINE.match(line)
        if m:
            flush_para(); close_list()
            out.append(f"<!--attach:{m.group(2)}-->")
            i += 1
            continue

        m = _HEADING.match(stripped)
        if m:
            flush_para(); close_list()
            level = min(len(m.group(1)) + 1, 4)  # card title is the h1; keep sections at h2+
            out.append(f"<h{level}>{inline(m.group(2))}</h{level}>")
            i += 1
            continue

        if _HR.match(stripped):
            flush_para(); close_list()
            out.append("<hr>")
            i += 1
            continue

        if stripped.startswith(">"):
            flush_para(); close_list()
            quote: List[str] = []
            while i < len(lines) and lines[i].strip().startswith(">"):
                quote.append(lines[i].strip()[1:].strip())
                i += 1
            out.append("<blockquote>" + inline(" ".join(quote)) + "</blockquote>")
            continue

        mb, mn = _BULLET.match(line), _NUMBERED.match(line)
        if mb or mn:
            flush_para()
            tag = "ul" if mb else "ol"
            if list_tag != tag:
                close_list()
                out.append(f"<{tag}>")
                list_tag = tag
            item = (mb or mn).group(1)
            mt = _TASK.match(item)
            if mt:
                box = "☑" if mt.group(1).lower() == "x" else "☐"
                out.append(f"<li>{box} {inline(mt.group(2))}</li>")
            else:
                out.append(f"<li>{inline(item)}</li>")
            i += 1
            continue

        close_list()
        para.append(stripped)
        i += 1

    flush_para(); close_list()
    return "\n".join(out)
