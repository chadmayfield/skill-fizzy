"""Thin HTTP client for the Fizzy API (stdlib only).

- Every request is scoped to the one configured account; there is no way to
  address another account through this client.
- Lists are followed through every page (Link: <...>; rel="next").
- Create calls return the new object's id parsed from the Location header.
- dry_run=True sends GETs normally but prints writes instead of sending them.
- Every write is appended to an action log (method, path, status; never the token).
"""

from __future__ import annotations

import json
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional, Tuple

from .config import Config


class ApiError(Exception):
    def __init__(self, status: int, method: str, path: str, body: str):
        self.status, self.method, self.path, self.body = status, method, path, body
        detail = body.strip()[:300]
        super().__init__(f"{method} {path} -> HTTP {status}" + (f": {detail}" if detail else ""))


_LINK_NEXT = re.compile(r'<([^>]+)>\s*;\s*rel="?next"?')


def next_link(link_header: Optional[str]) -> Optional[str]:
    """Return the rel="next" URL from a Link header, if any."""
    if not link_header:
        return None
    m = _LINK_NEXT.search(link_header)
    return m.group(1) if m else None


def id_from_location(location: Optional[str]) -> Optional[str]:
    """Last path segment of a Location header, without a .json suffix."""
    if not location:
        return None
    path = urllib.parse.urlparse(location).path.rstrip("/")
    last = path.rsplit("/", 1)[-1]
    return last[:-5] if last.endswith(".json") else last


class Client:
    def __init__(self, config: Config, dry_run: bool = False, opener=None, out=sys.stderr):
        self.cfg = config
        self.dry_run = dry_run
        self._opener = opener or urllib.request.build_opener()
        self._token: Optional[str] = None
        self._out = out

    # -- low level -------------------------------------------------------
    def _auth(self) -> str:
        if self._token is None:
            self._token = self.cfg.resolve_token()
        return self._token

    def url(self, path: str, account_scoped: bool = True) -> str:
        if path.startswith("http://") or path.startswith("https://"):
            return path
        prefix = f"/{self.cfg.account}" if account_scoped else ""
        return f"{self.cfg.url}{prefix}{path}"

    def request(self, method: str, path: str, body: Any = None, account_scoped: bool = True,
                raw: Optional[bytes] = None, headers: Optional[Dict[str, str]] = None,
                auth: bool = True) -> Tuple[int, Dict[str, str], bytes]:
        url = self.url(path, account_scoped)
        is_write = method != "GET"
        if is_write and self.dry_run:
            shown = json.dumps(body)[:400] if body is not None else (f"<{len(raw)} bytes>" if raw else "")
            print(f"[dry-run] {method} {url} {shown}", file=self._out)
            return 0, {}, b""
        hdrs = {"Accept": "application/json", "User-Agent": "fizzy-skill"}
        data = raw
        if body is not None:
            data = json.dumps(body).encode()
            hdrs["Content-Type"] = "application/json"
        if headers:
            hdrs.update(headers)
        if auth:
            hdrs["Authorization"] = f"Bearer {self._auth()}"
        req = urllib.request.Request(url, data=data, method=method, headers=hdrs)
        try:
            with self._opener.open(req, timeout=60) as resp:
                status, rh, payload = resp.status, {k.lower(): v for k, v in resp.headers.items()}, resp.read()
        except urllib.error.HTTPError as exc:
            status, rh, payload = exc.code, {k.lower(): v for k, v in exc.headers.items()}, exc.read()
        if is_write:
            self._log(method, url, status)
        if status >= 400:
            raise ApiError(status, method, urllib.parse.urlparse(url).path, payload.decode("utf-8", "replace"))
        return status, rh, payload

    def _log(self, method: str, url: str, status: int) -> None:
        try:
            p = self.cfg.log_path.expanduser()
            p.parent.mkdir(parents=True, exist_ok=True)
            with p.open("a") as f:
                f.write(json.dumps({"t": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                                    "method": method, "path": urllib.parse.urlparse(url).path,
                                    "status": status}) + "\n")
        except OSError:
            pass

    # -- helpers ---------------------------------------------------------
    def get(self, path: str, account_scoped: bool = True) -> Any:
        _, _, payload = self.request("GET", path, account_scoped=account_scoped)
        return json.loads(payload) if payload else None

    def get_all(self, path: str) -> List[Any]:
        """GET a list endpoint, following every page."""
        items: List[Any] = []
        url: Optional[str] = self.url(path)
        while url:
            _, headers, payload = self.request("GET", url)
            page = json.loads(payload) if payload else []
            items.extend(page)
            url = next_link(headers.get("link"))
        return items

    def create(self, path: str, body: Any) -> Optional[str]:
        """POST and return the created object's id (from Location), or None in dry-run."""
        status, headers, _ = self.request("POST", path, body)
        return id_from_location(headers.get("location")) if status else None

    def write(self, method: str, path: str, body: Any = None) -> Any:
        _, _, payload = self.request(method, path, body)
        try:
            return json.loads(payload) if payload else None
        except json.JSONDecodeError:
            return None
