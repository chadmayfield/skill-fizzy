package fizzy

import (
	"net/http"
	"strings"
	"testing"
)

func TestPyIndentMatchesPython(t *testing.T) {
	// Expected strings come from python3: json.dumps(json.loads(raw), indent=2).
	raw := `{"z": 1, "a": {"n": [], "e": {}, "f": 1.0, "g": 1e-5, "h": 1e16, "i": 100000.0, "j": 12345678901234567890}, "t": "café a/b ☃ 😀 \" \\ \n \u007f \u0001 <&>", "l": [1, [2, 3], {"k": null, "b": true}]}`
	want := "{\n  \"z\": 1,\n  \"a\": {\n    \"n\": [],\n    \"e\": {},\n    \"f\": 1.0,\n    \"g\": 1e-05,\n    \"h\": 1e+16,\n    \"i\": 100000.0,\n    \"j\": 12345678901234567890\n  },\n  \"t\": \"caf\\u00e9 a/b \\u2603 \\ud83d\\ude00 \\\" \\\\ \\n \\u007f \\u0001 <&>\",\n  \"l\": [\n    1,\n    [\n      2,\n      3\n    ],\n    {\n      \"k\": null,\n      \"b\": true\n    }\n  ]\n}"
	got, err := PyIndent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	// Raw UTF-8 in the input (not escaped) is escaped on output too.
	got, _ = PyIndent([]byte("[\"café/\U0001F600\"]"))
	if string(got) != "[\n  \"caf\\u00e9/\\ud83d\\ude00\"\n]" {
		t.Errorf("utf8: %q", got)
	}
	if got, _ := PyIndent([]byte(`[]`)); string(got) != "[]" {
		t.Errorf("empty list %q", got)
	}
	if got, _ := PyIndent([]byte(`[{"a":1}]`)); string(got) != "[\n  {\n    \"a\": 1\n  }\n]" {
		t.Errorf("list of object %q", got)
	}
	if _, err := PyIndent([]byte(`{"a":`)); err == nil {
		t.Error("expected error for truncated JSON")
	}
}

func TestRawKeptByTypes(t *testing.T) {
	src := `{"zeta":1,"number":5,"alpha":"café","surprise":{"x":[1,2]},"board":{"id":"B1","name":"H","extra":true},"column":{"id":"C1","name":"N","odd":1}}`
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.Write([]byte(src)) })
	c, err := f.Card(5)
	if err != nil {
		t.Fatal(err)
	}
	if string(c.Raw()) != src {
		t.Errorf("card raw %s", c.Raw())
	}
	if c.Number != 5 || c.Board.ID != "B1" || c.Column == nil || c.Column.ID != "C1" {
		t.Errorf("fields not decoded: %+v", c)
	}
	if string(c.Board.Raw()) != `{"id":"B1","name":"H","extra":true}` {
		t.Errorf("board raw %s", c.Board.Raw())
	}
	if string(c.Column.Raw()) != `{"id":"C1","name":"N","odd":1}` {
		t.Errorf("column raw %s", c.Column.Raw())
	}
	if !strings.HasPrefix(string(c.Raw()), `{"zeta":1,"number":5`) {
		t.Error("key order lost")
	}
}

func TestListsKeepRawPerElement(t *testing.T) {
	body := `[{"z":1,"id":"B1","name":"A","q":2},{"id":"B2","name":"B","k":null}]`
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.Write([]byte(body)) })
	boards, err := f.Boards()
	if err != nil || len(boards) != 2 {
		t.Fatalf("%v %v", boards, err)
	}
	if string(boards[0].Raw()) != `{"z":1,"id":"B1","name":"A","q":2}` || string(boards[1].Raw()) != `{"id":"B2","name":"B","k":null}` {
		t.Errorf("%s | %s", boards[0].Raw(), boards[1].Raw())
	}
	f2, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		w.Write([]byte(`[{"id":"T1","title":"buy","created_at":"2026-10-01T00:00:00Z","url":"u","x":1}]`))
	})
	tags, err := f2.Tags()
	if err != nil || tags[0].Title != "buy" || tags[0].CreatedAt != "2026-10-01T00:00:00Z" || tags[0].URL != "u" ||
		string(tags[0].Raw()) != `{"id":"T1","title":"buy","created_at":"2026-10-01T00:00:00Z","url":"u","x":1}` {
		t.Errorf("%+v %v", tags, err)
	}
}

func TestErrorTextsMatchPython(t *testing.T) {
	// Expected strings come from running the Python fz's api.py (removed in 562a4a9).
	boards := `[{"id":"B1","name":"Home"},{"id":"B2","name":"it's"},{"id":"B3","name":"Dup"},{"id":"B4","name":"dup"}]`
	cols := `[{"id":"C1","name":"Now"},{"id":"C2","name":"Café"}]`
	f, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) {
		if strings.HasSuffix(r.URL.Path, "/columns") {
			w.Write([]byte(cols))
		} else {
			w.Write([]byte(boards))
		}
	})
	check := func(name string, err error, want string) {
		t.Helper()
		if err == nil || err.Error() != want {
			t.Errorf("%s: got %v\nwant %s", name, err, want)
		}
	}
	_, err := f.Board("zzz")
	check("missing", err, "no board named 'zzz' (have: Home, it's, Dup, dup)")
	_, err = f.Board(`it's "x"`)
	check("both quotes", err, `no board named 'it\'s "x"' (have: Home, it's, Dup, dup)`)
	_, err = f.Board("DUP")
	check("ambiguous", err, "2 boards are named 'DUP'; use the board id")
	_, err = f.Board("café\n\\")
	check("escapes", err, `no board named 'café\n\\' (have: Home, it's, Dup, dup)`)
	_, err = f.Column("B1", "later")
	check("column", err, "no column 'later' (have: Now, Café)")
	_, err = f.Column("B1", "l'a")
	check("column dq", err, `no column "l'a" (have: Now, Café)`)
	_, err = f.CreateBoard("x", 5, nil)
	check("postpone", err, "auto-postpone must be one of (3, 7, 11, 30, 90, 365)")
	err = f.UpdateCard(1, CardFields{})
	check("nothing", err, "nothing to update")
	f0, _, _, _ := newFizzy(t, false, func(w http.ResponseWriter, r *http.Request, b []byte) { w.Write([]byte(`[]`)) })
	_, err = f0.Board("a")
	check("no boards", err, "no board named 'a' (have: )")
}
