# AGENTS.md for skill-fizzy

This file is for anyone changing this repo, people and coding agents alike.
`CLAUDE.md` holds one line, `@AGENTS.md`, so Claude Code reads this file too.

## What this repo is

This repo holds `fz`, a Go command-line client for the Fizzy API, and the
fizzy-cards Claude Code skill that drives it. The binary is built from
`cmd/fz`, and the commands live in `internal/cli/`. The skill is
`skill/SKILL.md` plus `skill/templates/card.md`, and it needs only `fz` on the
PATH.

The user guide is `docs/USERGUIDE.md`. When you change what a command does,
change the guide and the command's help text in the same commit.

## Layout

| Path | What it holds |
|---|---|
| `cmd/fz/` | `main.go`, which calls `cli.Execute` with the version, commit and date the Makefile stamps in. |
| `internal/cli/` | The cobra command tree. One file per command group, plus the fz 0.1 rewrites (`legacy.go`), the draft sync (`sync.go`, `fingerprint.go`) and the output helpers (`output.go`). |
| `internal/client/` | The HTTP client. Account scoping, pagination, dry run and the action log. |
| `internal/config/` | Loading `config.json` and the `FIZZY_*` variables, running the token command, and `SplitWords`, the shell-style word splitter. |
| `internal/fizzy/` | The Fizzy operations built on the client, the API types, and the Python-style JSON printing. |
| `internal/draft/` | Parsing card drafts, finding their images, writing `card:` and `synced:` back, and planning step changes. |
| `internal/markdown/` | Converting draft Markdown to the HTML Fizzy stores. |
| `internal/richtext/` | Small edits to Fizzy rich text, used by `card edit --replace`. |
| `internal/review/` | Gathering and formatting `fz review`. |
| `internal/docscheck/` | Tests that keep the Markdown docs honest. |
| `skill/` | The Claude Code skill. Users link this folder into `~/.claude/skills/`. |
| `examples/hooks/` | A Python SessionStart hook and its README. |
| `internal/*/testdata/` | Golden files. See [Tests that pin behavior](#tests-that-pin-behavior). |

## Build and test

`go.mod` sets Go 1.27.2. CI reads the Go version from `go.mod`.

- `make build` writes `./fz`.
- `make test` runs `go test -race -count=1 ./...`.
- `make lint` runs `go vet ./...`.
- `gofmt -l .` must print nothing.
- `make install` copies `./fz` to `~/.local/bin/fz`. `make man` writes the man pages to `~/.local/share/man`.
- `make vuln` runs `govulncheck ./...`, if you have it installed.
- `make tidy` runs `go mod tidy`.
- Releases: push a `v*` tag. `.github/workflows/release.yml` runs the tests, runs `make dist`, attests the archives and publishes the GitHub release.
- CI runs the gofmt check, `go vet`, the race tests and a build, plus `govulncheck` as advisory only.

## Adding a command

Each command lives in its own file under `internal/cli/` and registers itself by calling `register()` from that file's `init()`. Nobody edits a central list. Parallel worktrees depend on this, because two branches that both edit one list conflict on merge. `NewRoot()` in `internal/cli/root.go` applies every registered function.

`internal/cli/comments.go` is a short example to copy. Wrap the run function
in `action()`, wrap the argument check in `args()` so a bad argument exits 2,
and print results with `emit()` so `--json` works.

`internal/cli/help_test.go` checks every command:

- Every runnable command has a `Short`, a `Long` and an `Example`.
- A command group has a `Short`.
- Every example line starts with `fz `, resolves to the command it sits on,
  and has flags and arguments that parse.
- No help text, flag help or usage line contains an em dash.
- `wantCommands` lists the commands that must exist. Add a new command to it.

`internal/docscheck` also parses every `fz` line in the code blocks of
`README.md`, `docs/USERGUIDE.md` and `AGENTS.md`, so a renamed command or flag
fails the build until the docs catch up.

## Tests that pin behavior

- **Parity goldens.** `internal/cli/testdata/parity/python-fz.json` was
  captured from the Python fz on 2026-10-17. The Python fz is no longer in the
  repo, so the file can never be regenerated. When a parity test fails, fix
  the Go code, not the golden file.
- **Markdown and draft goldens.** The files in `internal/markdown/testdata/`
  and `internal/draft/testdata/` were written by the Python converter and
  draft parser. The same rule applies: fix the Go code.
- **Live shapes.** `internal/fizzy/shapes_test.go` decodes JSON with the key
  names and value types a real Fizzy server sends, filled with invented
  gate and hinge values.
- **Docs checks.** `internal/docscheck` checks the skill's template paths, the
  em dash rule, the `fz` lines in the docs, and that `CLAUDE.md` is exactly
  `@AGENTS.md`.
- **Private text in man pages.** `TestManNoPrivateText` fails if a man page
  holds a home directory, an `@` or `github.com`. Set `FZ_PRIVATE_PATTERNS`
  to a comma-separated list to check more strings. Keep that list in your
  environment, never in the repo.

## Rules

### Verify, never assume

- Read the code before you describe it, and run the binary before you
  document a flag. Every flag in the docs must appear in `--help`.
- Check a library's API in its docs or source before you use it.
- If something is unclear, stop and ask. Do not guess.

### Private data

- No server URLs, account ids, tokens, real card titles or personal paths in
  code, tests, docs or commits.
- The server URL comes from config or environment only. No URL, account, token or server name appears in code, tests, docs or commit messages. Tests use `httptest` servers and `fizzy.example.com`.
- Examples and fixtures use neutral data, never card titles, step ids or board ids from a real server. Use card 87, step id `03abc123def456ghi789jkl0mn` and the gate and hinge theme.
- Never print or log the token. The action log holds time, method, path and status only.

### Behavior every command keeps

- Every write honors `--dry-run`. Every delete needs `--yes`. So does publishing a board's public link and changing who can access a board. Changing a card's drafted or published status with `card edit --status` does not.
- Old command forms keep working.
- Dependencies: cobra and cobra/doc only. Ask Chad before adding any other.

### Writing

- Help text, README, man page and commit messages follow the unslop rules: no em dashes, plain words, sentence case. No emojis.
- The same rules cover `docs/USERGUIDE.md`, this file and code comments. One
  idea per sentence.

### Git

- Feature work happens on a branch and merges to `main` by pull request. Never commit to `main` directly.
- Nothing under `docs/superpowers/` or `.superpowers/` is committed. Those
  folders hold private plans. Stage files by explicit path.

## Commits

Every commit ends with a co-author trailer for the model that wrote it, then
the session link when one exists.

    Co-Authored-By: <the model that wrote the commit> <noreply@anthropic.com or the vendor's address>

For example:

    Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
    Claude-Session: <session URL>

Name the model that actually wrote the commit.
