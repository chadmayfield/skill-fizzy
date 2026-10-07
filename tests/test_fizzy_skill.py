import io
import json
import tempfile
import unittest
from pathlib import Path

from fizzy_skill import draft, markdown
from fizzy_skill.api import Fizzy
from fizzy_skill.client import Client, id_from_location, next_link
from fizzy_skill.config import Config


class FakeResponse:
    def __init__(self, status, body=b"", headers=None):
        self.status = status
        self._body = body if isinstance(body, bytes) else json.dumps(body).encode()
        self.headers = headers or {}

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


class FakeOpener:
    """Routes (method, url) to canned responses and records every request."""

    def __init__(self, routes):
        self.routes = routes
        self.calls = []

    def open(self, req, timeout=None):
        self.calls.append((req.get_method(), req.full_url, req.data))
        key = (req.get_method(), req.full_url)
        if key not in self.routes:
            raise AssertionError(f"unexpected request {key}")
        r = self.routes[key]
        return r(req) if callable(r) else r


def make_client(routes, dry_run=False, tmp=None):
    cfg = Config(url="https://fz.test", account="1", token="T")
    if tmp:
        cfg.log_path = Path(tmp) / "log"
    opener = FakeOpener(routes)
    return Client(cfg, dry_run=dry_run, opener=opener, out=io.StringIO()), opener


class TestHelpers(unittest.TestCase):
    def test_next_link(self):
        self.assertEqual(next_link('<https://x/1/cards?page=2>; rel="next"'), "https://x/1/cards?page=2")
        self.assertIsNone(next_link('<https://x/1/cards?page=1>; rel="prev"'))
        self.assertIsNone(next_link(None))

    def test_id_from_location(self):
        self.assertEqual(id_from_location("/1/boards/03abc.json"), "03abc")
        self.assertEqual(id_from_location("https://fz.test/1/cards/87"), "87")
        self.assertIsNone(id_from_location(None))


class TestClient(unittest.TestCase):
    def test_get_all_follows_pages(self):
        routes = {
            ("GET", "https://fz.test/1/tags"): FakeResponse(200, [{"title": "a"}], {"Link": '<https://fz.test/1/tags?page=2>; rel="next"'}),
            ("GET", "https://fz.test/1/tags?page=2"): FakeResponse(200, [{"title": "b"}]),
        }
        with tempfile.TemporaryDirectory() as t:
            c, _ = make_client(routes, tmp=t)
            self.assertEqual([x["title"] for x in c.get_all("/tags")], ["a", "b"])

    def test_dry_run_sends_no_writes(self):
        with tempfile.TemporaryDirectory() as t:
            c, opener = make_client({}, dry_run=True, tmp=t)
            self.assertIsNone(c.create("/boards", {"board": {"name": "x"}}))
            self.assertEqual(opener.calls, [])

    def test_token_header_and_log(self):
        routes = {("POST", "https://fz.test/1/boards"): FakeResponse(201, b"", {"Location": "/1/boards/B1.json"})}
        with tempfile.TemporaryDirectory() as t:
            c, opener = make_client(routes, tmp=t)
            self.assertEqual(c.create("/boards", {"board": {"name": "x"}}), "B1")
            log = (Path(t) / "log").read_text()
            self.assertIn('"status": 201', log)
            self.assertNotIn("T", json.loads(log)["path"])  # token never logged


class TestApi(unittest.TestCase):
    def test_board_created_then_postpone_set(self):
        routes = {
            ("POST", "https://fz.test/1/boards"): FakeResponse(201, b"", {"Location": "/1/boards/B1.json"}),
            ("PUT", "https://fz.test/1/boards/B1"): FakeResponse(200, {}),
        }
        with tempfile.TemporaryDirectory() as t:
            c, opener = make_client(routes, tmp=t)
            Fizzy(c).create_board("Home", 365)
            first_body = json.loads(opener.calls[0][2])
            self.assertNotIn("auto_postpone_period_in_days", first_body["board"])
            self.assertEqual(json.loads(opener.calls[1][2])["board"]["auto_postpone_period_in_days"], 365)

    def test_add_tag_is_idempotent(self):
        routes = {("GET", "https://fz.test/1/cards/5"): FakeResponse(200, {"tags": ["buy"]})}
        with tempfile.TemporaryDirectory() as t:
            c, opener = make_client(routes, tmp=t)
            self.assertFalse(Fizzy(c).add_tag(5, "#Buy"))
            self.assertEqual([m for m, _, _ in opener.calls], ["GET"])  # no toggle sent

    def test_move_to_named_column(self):
        routes = {
            ("GET", "https://fz.test/1/cards/5"): FakeResponse(200, {"board": {"id": "B1"}}),
            ("GET", "https://fz.test/1/boards/B1/columns"): FakeResponse(200, [{"id": "C1", "name": "Now"}]),
            ("POST", "https://fz.test/1/cards/5/triage"): FakeResponse(204),
        }
        with tempfile.TemporaryDirectory() as t:
            c, opener = make_client(routes, tmp=t)
            Fizzy(c).move_card(5, "now")
            self.assertEqual(json.loads(opener.calls[-1][2]), {"column_id": "C1"})

    def test_card_search_is_url_encoded(self):
        # A raw space in the query made http.client refuse the URL.
        url = "https://fz.test/1/cards?board_ids%5B%5D=B1&terms%5B%5D=first+aid&terms%5B%5D=a%26b"
        routes = {("GET", url): FakeResponse(200, [])}
        with tempfile.TemporaryDirectory() as t:
            c, opener = make_client(routes, tmp=t)
            Fizzy(c).cards("B1", **{"terms[]": ["first aid", "a&b"]})
            self.assertEqual(opener.calls[0][1], url)


class TestUpload(unittest.TestCase):
    def test_prefers_attachable_sgid(self):
        info = {"signed_id": "BLOB", "attachable_sgid": "ATTACH",
                "direct_upload": {"url": "https://store.test/put", "headers": {"Content-Type": "image/png"}}}
        routes = {
            ("POST", "https://fz.test/1/rails/active_storage/direct_uploads"): FakeResponse(200, info),
            ("PUT", "https://store.test/put"): FakeResponse(204),
        }
        with tempfile.TemporaryDirectory() as t:
            f = Path(t) / "a.png"
            f.write_bytes(b"png")
            c, opener = make_client(routes, tmp=t)
            self.assertEqual(Fizzy(c).upload(f), "ATTACH")
            put = [r for r in opener.calls if r[0] == "PUT"][0]
            self.assertEqual(put[2], b"png")


class TestMarkdown(unittest.TestCase):
    def test_basics(self):
        h = markdown.to_html("## Goal\nShip **it** by `Friday`.\n\n- [x] one\n- [ ] two\n\nSee [docs](https://ex.com/a).")
        self.assertIn("<h3>Goal</h3>", h)
        self.assertIn("<strong>it</strong>", h)
        self.assertIn("<code>Friday</code>", h)
        self.assertIn("<li>☑ one</li>", h)
        self.assertIn('<a href="https://ex.com/a">docs</a>', h)

    def test_escapes_html(self):
        self.assertIn("&lt;script&gt;", markdown.to_html("<script>x</script>"))

    def test_image_placeholder(self):
        self.assertIn("<!--attach:pic.jpg-->", markdown.to_html("![a photo](pic.jpg)"))


DRAFT = """---
board: Garden
column: Next
tags: build, #Buy
---
# Fix the gate

## Goal
Shocks installed.

## Links & media
- [Guide](https://example.com/guide)
![rear](rear.jpg)

## Steps
- [ ] Measure
- [x] Order
"""


class TestDraft(unittest.TestCase):
    def test_parse_and_record(self):
        with tempfile.TemporaryDirectory() as t:
            p = Path(t) / "rear.md"
            p.write_text(DRAFT)
            (Path(t) / "rear.jpg").write_bytes(b"\xff\xd8")
            d = draft.parse(p)
            self.assertEqual((d.title, d.board, d.column), ("Fix the gate", "Garden", "Next"))
            self.assertEqual(d.tags, ["build", "buy"])
            self.assertEqual(d.steps, [("Measure", False), ("Order", True)])
            self.assertEqual(d.links(), ["https://example.com/guide"])
            self.assertEqual(d.resolve_image("rear.jpg", Path(t) / "inbox"), Path(t) / "rear.jpg")
            html = d.description_html({"rear.jpg": "SG"})
            self.assertIn('<action-text-attachment sgid="SG">', html)
            self.assertNotIn("Measure", html)  # steps are not in the description
            draft.record_card(p, 42)
            self.assertEqual(draft.parse(p).card, 42)

    def test_requires_board_and_title(self):
        with tempfile.TemporaryDirectory() as t:
            p = Path(t) / "x.md"
            p.write_text("---\ncolumn: Now\n---\n# T\n")
            with self.assertRaises(draft.DraftError):
                draft.parse(p)
            p.write_text("---\nboard: B\n---\nno title\n")
            with self.assertRaises(draft.DraftError):
                draft.parse(p)

    def test_bad_step_line_rejected(self):
        with tempfile.TemporaryDirectory() as t:
            p = Path(t) / "x.md"
            p.write_text("---\nboard: B\n---\n# T\n## Steps\n- not a checkbox\n")
            with self.assertRaises(draft.DraftError):
                draft.parse(p)


if __name__ == "__main__":
    unittest.main()
