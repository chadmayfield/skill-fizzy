# CLAUDE.md for skill-fizzy

This repo holds `fz`, a Go command-line client for the Fizzy API, and the fizzy-cards Claude Code skill that drives it. The binary is built from `cmd/fz`, and the commands live in `internal/cli/`. The skill is `skill/SKILL.md` plus `skill/templates/card.md`, and it needs only `fz` on the PATH.

## Build and test

- `make build` writes `./fz`.
- `make test` runs `go test -race -count=1 ./...`.
- `make lint` runs `go vet ./...`.
- `gofmt -l .` must print nothing.
- `make install` copies `./fz` to `~/.local/bin/fz`. `make man` writes the man pages to `~/.local/share/man`.
- CI runs the gofmt check, `go vet`, the race tests and a build, plus `govulncheck` as advisory only.
- The golden files in `internal/cli/testdata/parity/`, `internal/markdown/testdata/` and `internal/draft/testdata/` were captured from the old Python fz, which is no longer in the repo. When a test against them fails, fix the Go code, not the golden file.

## Adding a command

Each command lives in its own file under `internal/cli/` and registers itself by calling `register()` from that file's `init()`. Nobody edits a central list. Parallel worktrees depend on this, because two branches that both edit one list conflict on merge. `NewRoot()` in `internal/cli/root.go` applies every registered function.

## Conventions

- Dependencies: cobra and cobra/doc only. Ask Chad before adding any other.
- The server URL comes from config or environment only. No URL, account, token or server name appears in code, tests, docs or commit messages. Tests use `httptest` servers and `fizzy.example.com`.
- Examples and fixtures use neutral data, never card titles, step ids or board ids from a real server.
- Never print or log the token. The action log holds time, method, path and status only.
- Every write honors `--dry-run`. Every delete, and anything that publishes or changes access, needs `--yes`.
- Old command forms keep working.
- Help text, README, man page and commit messages follow the unslop rules: no em dashes, plain words, sentence case. No emojis.
- Feature work happens on a branch and merges to `main` by pull request. Never commit to `main` directly.
- Never `git add docs/` or `.superpowers/`. Stage files by explicit path.

## Commits

Every commit ends with a co-author trailer for the Claude model that wrote it,
then the session link.

    Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
    Claude-Session: <session URL>
