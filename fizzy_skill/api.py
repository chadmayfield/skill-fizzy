"""High-level Fizzy operations built on Client.

Notes on Fizzy behaviour this module works around (verified against Fizzy
source and docs, 2026-10):
- Creating a board with auto_postpone_period_in_days in the same request can
  fail 422 "Account must exist"; boards are created by name, then updated.
- POST /cards/:n/taggings *toggles* a tag. add_tag/remove_tag read the card's
  current tags first so they are idempotent.
- Fizzy fixes the lanes "Not now" (left), "Maybe?" (triage), and "Done"
  (right); only custom columns can be created or reordered.
"""

from __future__ import annotations

import base64
import hashlib
import json
import mimetypes
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urlencode

from .client import Client

AUTO_POSTPONE_DAYS = (3, 7, 11, 30, 90, 365)

# Special column targets accepted by move_card
MAYBE, NOT_NOW, DONE = "maybe", "not-now", "done"


class FizzyError(Exception):
    pass


class Fizzy:
    def __init__(self, client: Client):
        self.c = client

    # -- boards & columns -----------------------------------------------
    def boards(self) -> List[Dict[str, Any]]:
        return self.c.get_all("/boards")

    def board(self, name_or_id: str) -> Dict[str, Any]:
        boards = self.boards()
        for b in boards:
            if b["id"] == name_or_id:
                return b
        matches = [b for b in boards if b["name"].lower() == name_or_id.lower()]
        if len(matches) == 1:
            return matches[0]
        if not matches:
            raise FizzyError(f"no board named {name_or_id!r} (have: {', '.join(b['name'] for b in boards)})")
        raise FizzyError(f"{len(matches)} boards are named {name_or_id!r}; use the board id")

    def create_board(self, name: str, auto_postpone_days: Optional[int] = None) -> Optional[str]:
        if auto_postpone_days is not None and auto_postpone_days not in AUTO_POSTPONE_DAYS:
            raise FizzyError(f"auto-postpone must be one of {AUTO_POSTPONE_DAYS}")
        board_id = self.c.create("/boards", {"board": {"name": name}})
        if board_id and auto_postpone_days is not None:
            self.c.write("PUT", f"/boards/{board_id}", {"board": {"auto_postpone_period_in_days": auto_postpone_days}})
        return board_id

    def columns(self, board_id: str) -> List[Dict[str, Any]]:
        return self.c.get_all(f"/boards/{board_id}/columns")

    def column(self, board_id: str, name_or_id: str) -> Dict[str, Any]:
        cols = self.columns(board_id)
        for col in cols:
            if col["id"] == name_or_id or col["name"].lower() == name_or_id.lower():
                return col
        raise FizzyError(f"no column {name_or_id!r} (have: {', '.join(c['name'] for c in cols)})")

    def create_column(self, board_id: str, name: str, color: Optional[str] = None) -> Optional[str]:
        body: Dict[str, Any] = {"name": name}
        if color:
            body["color"] = color if color.startswith("var(") else f"var(--color-card-{color})"
        return self.c.create(f"/boards/{board_id}/columns", {"column": body})

    # -- cards -------------------------------------------------------------
    def cards(self, board_id: Optional[str] = None, **filters: Any) -> List[Dict[str, Any]]:
        params = []
        if board_id:
            params.append(("board_ids[]", board_id))
        for k, v in filters.items():
            if v is None:
                continue
            for item in (v if isinstance(v, (list, tuple)) else [v]):
                params.append((k, item))
        query = urlencode(params)
        return self.c.get_all("/cards" + (f"?{query}" if query else ""))

    def card(self, number: int) -> Dict[str, Any]:
        return self.c.get(f"/cards/{int(number)}")

    def create_card(self, board_id: str, title: str, description_html: str = "",
                    status: Optional[str] = None) -> Optional[int]:
        body: Dict[str, Any] = {"title": title}
        if description_html:
            body["description"] = description_html
        if status:
            body["status"] = status
        number = self.c.create(f"/boards/{board_id}/cards", {"card": body})
        return int(number) if number else None

    def update_card(self, number: int, **fields: Any) -> Any:
        body = {k: v for k, v in fields.items() if v is not None}
        if not body:
            raise FizzyError("nothing to update")
        return self.c.write("PUT", f"/cards/{int(number)}", {"card": body})

    def move_card(self, number: int, target: str, board_id: Optional[str] = None) -> None:
        """target: a column name/id on the card's board, or maybe / not-now / done."""
        n = int(number)
        t = target.strip().lower().rstrip("?")
        if t == MAYBE:
            self.c.write("DELETE", f"/cards/{n}/triage")
        elif t in (NOT_NOW, "not now"):
            self.c.write("POST", f"/cards/{n}/not_now")
        elif t == DONE:
            self.c.write("POST", f"/cards/{n}/closure")
        else:
            bid = board_id or self.card(n)["board"]["id"]
            col = self.column(bid, target)
            self.c.write("POST", f"/cards/{n}/triage", {"column_id": col["id"]})

    def reopen_card(self, number: int) -> None:
        self.c.write("DELETE", f"/cards/{int(number)}/closure")

    def move_card_to_board(self, number: int, board_id: str) -> Any:
        return self.c.write("PUT", f"/cards/{int(number)}/board", {"board_id": board_id})

    def set_golden(self, number: int, golden: bool = True) -> None:
        self.c.write("POST" if golden else "DELETE", f"/cards/{int(number)}/goldness")

    def delete_card(self, number: int) -> None:
        self.c.write("DELETE", f"/cards/{int(number)}")

    # -- tags (idempotent over Fizzy's toggle) -----------------------------
    def _card_tags(self, number: int) -> List[str]:
        return [t.lower() for t in self.card(number).get("tags", [])]

    def add_tag(self, number: int, tag: str) -> bool:
        tag = tag.lstrip("#").strip().lower()
        if tag in self._card_tags(number):
            return False
        self.c.write("POST", f"/cards/{int(number)}/taggings", {"tag_title": tag})
        return True

    def remove_tag(self, number: int, tag: str) -> bool:
        tag = tag.lstrip("#").strip().lower()
        if tag not in self._card_tags(number):
            return False
        self.c.write("POST", f"/cards/{int(number)}/taggings", {"tag_title": tag})
        return True

    def tags(self) -> List[Dict[str, Any]]:
        return self.c.get_all("/tags")

    # -- steps & comments ----------------------------------------------------
    def add_step(self, number: int, content: str, completed: bool = False) -> Optional[str]:
        return self.c.create(f"/cards/{int(number)}/steps", {"step": {"content": content, "completed": completed}})

    def set_step(self, number: int, step_id: str, completed: bool) -> Any:
        return self.c.write("PUT", f"/cards/{int(number)}/steps/{step_id}", {"step": {"completed": completed}})

    def add_comment(self, number: int, body_html: str) -> Optional[str]:
        return self.c.create(f"/cards/{int(number)}/comments", {"comment": {"body": body_html}})

    # -- uploads (ActiveStorage direct upload) ------------------------------
    def upload(self, path: Path) -> Optional[str]:
        """Upload a file; return the sgid to embed as <action-text-attachment sgid=...>."""
        data = path.read_bytes()
        checksum = base64.b64encode(hashlib.md5(data).digest()).decode()  # noqa: S324 (ActiveStorage requires MD5)
        ctype = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
        blob = {"blob": {"filename": path.name, "byte_size": len(data), "checksum": checksum, "content_type": ctype}}
        if self.c.dry_run:
            self.c.request("POST", "/rails/active_storage/direct_uploads", blob)
            return "DRY-RUN-SGID"
        _, _, payload = self.c.request("POST", "/rails/active_storage/direct_uploads", blob)
        info = json.loads(payload)
        du = info["direct_upload"]
        # The storage URL is absolute for cloud services; a disk service may return a
        # site-relative path, which already carries any account prefix it needs.
        self.c.request("PUT", du["url"], raw=data, headers=du.get("headers", {}),
                       auth=False, account_scoped=False)
        # Rich text needs the *attachable* sgid (purpose "attachable"). Fizzy's API
        # docs say signed_id, but that has purpose "blob_id" and renders as a broken
        # attachment. Rails adds attachable_sgid to the blob JSON (ActionText::Attachable).
        return info.get("attachable_sgid") or info["signed_id"]
