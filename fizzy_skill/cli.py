"""fz: command-line client for the Fizzy API.

Every write accepts --dry-run (global) to print what would be sent instead.
Deleting needs --yes. Output is JSON with --json, otherwise short text.
"""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, List, Optional

from . import __version__
from . import config as config_mod
from . import draft as draft_mod
from .api import AUTO_POSTPONE_DAYS, Fizzy, FizzyError
from .client import ApiError, Client


def _print(args: argparse.Namespace, data: Any, text: str) -> None:
    if args.json:
        print(json.dumps(data, indent=2))
    else:
        print(text)


def _card_line(c: dict) -> str:
    col = (c.get("column") or {}).get("name") if isinstance(c.get("column"), dict) else None
    state = "done" if c.get("closed") else (col or "maybe")
    gold = " ★" if c.get("golden") else ""
    tags = f" #{' #'.join(c['tags'])}" if c.get("tags") else ""
    return f"#{c['number']:<5} [{state}] {c['title']}{gold}{tags}"


# -- commands ----------------------------------------------------------------
def cmd_boards(fz: Fizzy, args) -> int:
    boards = fz.boards()
    _print(args, boards, "\n".join(f"{b['id']}  {b['name']}  (postpone {b.get('auto_postpone_period_in_days')}d)" for b in boards))
    return 0


def cmd_board_create(fz: Fizzy, args) -> int:
    board_id = fz.create_board(args.name, args.postpone)
    cols = [c.strip() for c in (args.columns or "").split(",") if c.strip()]
    for name in cols:
        if board_id:
            fz.create_column(board_id, name)
    _print(args, {"id": board_id, "columns": cols}, f"board {args.name!r} created: {board_id}")
    return 0


def cmd_columns(fz: Fizzy, args) -> int:
    b = fz.board(args.board)
    cols = fz.columns(b["id"])
    _print(args, cols, "Not now → Maybe? → " + " → ".join(c["name"] for c in cols) + " → Done")
    return 0


def cmd_cards(fz: Fizzy, args) -> int:
    board_id = fz.board(args.board)["id"] if args.board else None
    cards = fz.cards(board_id, **{"indexed_by": args.state, "terms[]": args.search})
    _print(args, cards, "\n".join(_card_line(c) for c in cards) or "(no cards)")
    return 0


def cmd_show(fz: Fizzy, args) -> int:
    c = fz.card(args.number)
    if args.json:
        print(json.dumps(c, indent=2))
        return 0
    print(_card_line(c))
    print(f"board: {c['board']['name']}   url: {c.get('url')}")
    if c.get("description"):
        print("\n" + c["description"].strip())
    for s in c.get("steps") or []:
        print(f"  [{'x' if s.get('completed') else ' '}] {s.get('content')}")
    return 0


def cmd_create(fz: Fizzy, args) -> int:
    b = fz.board(args.board)
    desc = args.description or ""
    if args.description_file:
        desc = Path(args.description_file).read_text()
    from .markdown import to_html
    number = fz.create_card(b["id"], args.title, to_html(desc) if desc else "")
    if number:
        if args.column and args.column.lower().rstrip("?") != "maybe":
            fz.move_card(number, args.column, b["id"])
        for t in args.tag or []:
            fz.add_tag(number, t)
    _print(args, {"number": number}, f"card #{number} created on {b['name']}" if number else "dry-run: card not created")
    return 0


def cmd_move(fz: Fizzy, args) -> int:
    if args.board:
        fz.move_card_to_board(args.number, fz.board(args.board)["id"])
    if args.to:
        fz.move_card(args.number, args.to)
    _print(args, {"number": args.number}, f"card #{args.number} moved")
    return 0


def cmd_reopen(fz: Fizzy, args) -> int:
    fz.reopen_card(args.number)
    _print(args, {"number": args.number}, f"card #{args.number} reopened")
    return 0


def cmd_golden(fz: Fizzy, args) -> int:
    fz.set_golden(args.number, not args.off)
    _print(args, {"number": args.number, "golden": not args.off}, f"card #{args.number} golden={'off' if args.off else 'on'}")
    return 0


def cmd_tag(fz: Fizzy, args) -> int:
    changed = fz.remove_tag(args.number, args.tag) if args.remove else fz.add_tag(args.number, args.tag)
    verb = "removed" if args.remove else "added"
    _print(args, {"changed": changed}, f"tag {args.tag} {verb}" if changed else f"tag {args.tag} already {'absent' if args.remove else 'present'}")
    return 0


def cmd_step(fz: Fizzy, args) -> int:
    if args.check or args.uncheck:
        fz.set_step(args.number, args.check or args.uncheck, completed=bool(args.check))
        _print(args, {}, "step updated")
    else:
        sid = fz.add_step(args.number, args.content, completed=args.done)
        _print(args, {"id": sid}, f"step added: {sid}")
    return 0


def cmd_comment(fz: Fizzy, args) -> int:
    from .markdown import to_html
    cid = fz.add_comment(args.number, to_html(args.text))
    _print(args, {"id": cid}, f"comment added: {cid}")
    return 0


def cmd_delete(fz: Fizzy, args) -> int:
    c = fz.card(args.number)
    print(f"about to delete: {_card_line(c)} (board {c['board']['name']})", file=sys.stderr)
    if not args.yes:
        print("refusing without --yes", file=sys.stderr)
        return 2
    fz.delete_card(args.number)
    _print(args, {"deleted": args.number}, f"card #{args.number} deleted")
    return 0


def cmd_upload(fz: Fizzy, args) -> int:
    sgid = fz.upload(Path(args.file).expanduser())
    _print(args, {"sgid": sgid}, sgid or "")
    return 0


# -- drafts ----------------------------------------------------------------------
def _draft_paths(targets: List[str]) -> List[Path]:
    paths: List[Path] = []
    for t in targets:
        p = Path(t).expanduser()
        paths.extend(sorted(p.glob("*.md")) if p.is_dir() else [p])
    return paths


def check_links(urls: List[str], timeout: int = 15) -> List[tuple]:
    """Return (url, status-or-error) for each URL. GET, following redirects."""
    results = []
    for url in urls:
        req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0 (fizzy-skill link check)"})
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                results.append((url, r.status))
        except urllib.error.HTTPError as e:
            results.append((url, e.code))
        except Exception as e:  # DNS, TLS, timeout
            results.append((url, type(e).__name__))
    return results


def cmd_draft_check(fz: Optional[Fizzy], args, cfg) -> int:
    bad = 0
    for p in _draft_paths(args.drafts):
        try:
            d = draft_mod.parse(p)
            for img in d.images():
                d.resolve_image(img, cfg.inbox_path)
        except draft_mod.DraftError as e:
            print(f"✗ {p}: {e}")
            bad += 1
            continue
        line = f"✓ {p.name}: {d.title!r} → {d.board} / {d.column}, {len(d.steps)} steps, {len(d.images())} images"
        if d.card:
            line += f" (already created: #{d.card})"
        print(line)
        if args.links:
            for url, status in check_links(d.links()):
                ok = isinstance(status, int) and status < 400
                print(f"    {'ok ' if ok else 'BAD'} {status} {url}")
                bad += 0 if ok else 1
    return 1 if bad else 0


def cmd_draft_preview(args, cfg) -> int:
    for p in _draft_paths(args.drafts):
        d = draft_mod.parse(p)
        print(f"=== {d.title}\nboard: {d.board}   column: {d.column}   tags: {', '.join(d.tags) or '-'}   golden: {d.golden}")
        print(d.description_html())
        for content, done in d.steps:
            print(f"  [{'x' if done else ' '}] {content}")
        print()
    return 0


def apply_draft(fz: Fizzy, d: draft_mod.Draft, cfg) -> Optional[int]:
    board = fz.board(d.board)
    sgids = {img: fz.upload(d.resolve_image(img, cfg.inbox_path)) for img in d.images()}
    number = fz.create_card(board["id"], d.title, d.description_html(sgids))
    if not number:  # dry-run: show the follow-up calls too, against a placeholder card
        for content, done in d.steps:
            fz.add_step(0, content, done)
        for t in d.tags:
            print(f"[dry-run] tag new card: {t}", file=sys.stderr)
        if d.column.lower().rstrip("?") != "maybe":
            print(f"[dry-run] move new card to column: {d.column}", file=sys.stderr)
        if d.golden:
            print("[dry-run] mark new card golden", file=sys.stderr)
        return None
    for content, done in d.steps:
        fz.add_step(number, content, done)
    for t in d.tags:
        fz.add_tag(number, t)
    if d.column.lower().rstrip("?") != "maybe":
        fz.move_card(number, d.column, board["id"])
    if d.golden:
        fz.set_golden(number, True)
    return number


def cmd_draft_apply(fz: Fizzy, args, cfg) -> int:
    paths = _draft_paths(args.drafts)
    drafts = []
    for p in paths:  # validate everything first: all or nothing on parse errors
        d = draft_mod.parse(p)
        for img in d.images():
            d.resolve_image(img, cfg.inbox_path)
        drafts.append(d)
    for d in drafts:
        if d.card:
            print(f"skip {d.path.name}: already card #{d.card}")
            continue
        number = apply_draft(fz, d, cfg)
        if number:
            draft_mod.record_card(d.path, number)
            print(f"#{number} {d.title} → {d.board} / {d.column}   {cfg.url}/{cfg.account}/cards/{number}")
        else:
            print(f"[dry-run] {d.title} → {d.board} / {d.column}")
    return 0


# -- parser ------------------------------------------------------------------
def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="fz", description="Fizzy API client")
    p.add_argument("--version", action="version", version=f"fz {__version__}")
    p.add_argument("--dry-run", action="store_true", help="print writes instead of sending them")
    p.add_argument("--json", action="store_true", help="JSON output")
    p.add_argument("--config", help="config file (default ~/.config/fizzy-skill/config.json)")
    sub = p.add_subparsers(dest="cmd", required=True)

    sub.add_parser("boards", help="list boards").set_defaults(fn=cmd_boards)
    s = sub.add_parser("board-create", help="create a board")
    s.add_argument("name"); s.add_argument("--postpone", type=int, choices=AUTO_POSTPONE_DAYS)
    s.add_argument("--columns", help="comma-separated, left to right (e.g. Later,Next,Now)")
    s.set_defaults(fn=cmd_board_create)
    s = sub.add_parser("columns", help="list a board's columns"); s.add_argument("board"); s.set_defaults(fn=cmd_columns)

    s = sub.add_parser("cards", help="list cards (all pages)")
    s.add_argument("--board"); s.add_argument("--state", choices=["all", "maybe", "closed", "not_now", "stalled", "postponing_soon", "golden"])
    s.add_argument("--search"); s.set_defaults(fn=cmd_cards)
    s = sub.add_parser("show", help="show a card"); s.add_argument("number", type=int); s.set_defaults(fn=cmd_show)

    s = sub.add_parser("create", help="create a card")
    s.add_argument("board"); s.add_argument("title")
    s.add_argument("--description", help="Markdown text"); s.add_argument("--description-file")
    s.add_argument("--column", help="column name, or maybe (default)"); s.add_argument("--tag", action="append")
    s.set_defaults(fn=cmd_create)

    s = sub.add_parser("move", help="move a card: --to <column|maybe|not-now|done> and/or --board")
    s.add_argument("number", type=int); s.add_argument("--to"); s.add_argument("--board"); s.set_defaults(fn=cmd_move)
    s = sub.add_parser("reopen", help="reopen a done card"); s.add_argument("number", type=int); s.set_defaults(fn=cmd_reopen)
    s = sub.add_parser("golden", help="mark golden (or --off)"); s.add_argument("number", type=int)
    s.add_argument("--off", action="store_true"); s.set_defaults(fn=cmd_golden)
    s = sub.add_parser("tag", help="add a tag (or --remove); idempotent")
    s.add_argument("number", type=int); s.add_argument("tag"); s.add_argument("--remove", action="store_true"); s.set_defaults(fn=cmd_tag)
    s = sub.add_parser("step", help="add a step, or --check/--uncheck STEP_ID")
    s.add_argument("number", type=int); s.add_argument("content", nargs="?"); s.add_argument("--done", action="store_true")
    s.add_argument("--check"); s.add_argument("--uncheck"); s.set_defaults(fn=cmd_step)
    s = sub.add_parser("comment", help="add a comment (Markdown)"); s.add_argument("number", type=int); s.add_argument("text"); s.set_defaults(fn=cmd_comment)
    s = sub.add_parser("delete", help="delete a card (needs --yes)"); s.add_argument("number", type=int)
    s.add_argument("--yes", action="store_true"); s.set_defaults(fn=cmd_delete)
    s = sub.add_parser("upload", help="upload a file, print its sgid"); s.add_argument("file"); s.set_defaults(fn=cmd_upload)

    d = sub.add_parser("draft", help="work with Markdown card drafts")
    dsub = d.add_subparsers(dest="dcmd", required=True)
    s = dsub.add_parser("check", help="validate drafts (and --links)"); s.add_argument("drafts", nargs="+")
    s.add_argument("--links", action="store_true"); s.set_defaults(dfn="check")
    s = dsub.add_parser("preview", help="show the card a draft would create"); s.add_argument("drafts", nargs="+"); s.set_defaults(dfn="preview")
    s = dsub.add_parser("apply", help="create cards from drafts (files or folders)"); s.add_argument("drafts", nargs="+"); s.set_defaults(dfn="apply")
    return p


def main(argv: Optional[List[str]] = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        cfg = config_mod.load(args.config)
        if args.cmd == "draft" and args.dfn == "preview":
            return cmd_draft_preview(args, cfg)
        fz = Fizzy(Client(cfg, dry_run=args.dry_run))
        if args.cmd == "draft":
            return {"check": cmd_draft_check, "apply": cmd_draft_apply}[args.dfn](fz, args, cfg)
        if args.cmd == "step" and not (args.content or args.check or args.uncheck):
            print("step: give content, or --check/--uncheck STEP_ID", file=sys.stderr)
            return 2
        return args.fn(fz, args)
    except (config_mod.ConfigError, FizzyError, draft_mod.DraftError, ApiError) as e:
        print(f"fz: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
