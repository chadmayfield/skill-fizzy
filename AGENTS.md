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
| `internal/cli/cards_extra.go` | `card assign`, `unassign`, `watch`, `unwatch`, `pin`, `unpin`, `pins`, `boost`, `boosts`, `unboost` and `image`. |
| `internal/cli/comments_extra.go` | The `comment` commands after `add`: `list`, `show`, `edit`, `delete`, `react`, `reactions` and `unreact`. |
| `internal/cli/boards_extra.go` | `board show`, `edit`, `delete`, `publish`, `unpublish` and `access`. |
| `internal/cli/columns.go` | `column show`, `create`, `edit`, `delete` and `cards`. |
| `internal/cli/people.go` | `tag list`, the `user` commands and `whoami`. |
| `internal/cli/activity.go` | `activity list` and the `notification` commands. |
| `internal/cli/account.go` | `account show`, `account postpone` and the `export` commands. |
| `internal/cli/webhooks.go` | The `webhook` commands. |
| `internal/client/` | The HTTP client. Account scoping, pagination, dry run and the action log. |
| `internal/config/` | Loading `config.json` and the `FIZZY_*` variables, running the token command, and `SplitWords`, the shell-style word splitter. |
| `internal/fizzy/` | The Fizzy operations built on the client, the API types, and the Python-style JSON printing. |
| `internal/fizzy/cards_extra.go` | Assignments, watching, pins, boosts, the header image and the tag lookup for `card list --tag`. |
| `internal/fizzy/comments.go` | Comments and their reactions. |
| `internal/fizzy/boards_extra.go` | Board details, edits, access and publishing, and the calls that read, change and delete one column. |
| `internal/fizzy/users.go` | Users, and `MatchUser`, the one function that turns a USER argument into a user. |
| `internal/fizzy/account.go` | The account, identity, timezone, notifications, activity and exports. |
| `internal/fizzy/webhooks.go` | Webhooks and their deliveries, and the URL and action checks. |
| `internal/fizzy/refusal.go` | `Refusal`, the error for a check that stops a change before it is sent. The CLI exits 2 on it. |
| `internal/draft/` | Parsing card drafts, finding their images, writing `card:` and `synced:` back, and planning step changes. |
| `internal/markdown/` | Converting draft Markdown to the HTML Fizzy stores. |
| `internal/richtext/` | Small edits to Fizzy rich text, used by `card edit --replace`. |
| `internal/review/` | Gathering and formatting `fz review`. |
| `internal/testenv/` | `Main`, which every test package that builds a client or runs commands calls from its `TestMain`. It gives the tests a temporary HOME, a missing config file and no `FIZZY_*` variables, so no test reads the real config or reaches a real server. A test that needs a variable sets it with `t.Setenv`. The Go caches and the go env file keep their real places. A test fails if a package that can reach `internal/config` has tests but no `testenv.Main`. |
| `internal/docscheck/` | Tests that keep the Markdown docs honest. |
| `internal/repocheck/` | A test that fails if git tracks a private planning or agent file. |
| `internal/releasecheck/` | Tests for the release build and the tag guard script. |
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
- Releases: push a tag named `vMAJOR.MINOR.PATCH`, with an optional `-suffix`, on a commit that is already on `main`.
  `.github/workflows/release.yml` first runs `scripts/release-guard.sh`, which fails any other tag name or a commit not on `origin/main`, and records the kind in `GITHUB_OUTPUT`.
  The guard rejects leading zeros, line breaks, and a suffix with an empty or zero-padded numeric part, such as `-rc.01` or `-a..b`.
  It then runs the tests, runs `make dist` and publishes the GitHub release. A tag with a suffix is published as a prerelease.
  The workflow has two jobs. `build` runs the guard, the tests and `make dist` with read-only contents, and uploads the archives.
  `publish` runs no repo code: it downloads those archives, attests them and creates the release, and only it has the write, id-token and attestation scopes.
  The build job turns off setup-go's cache, so it builds with empty Go caches.
  The archives are attested only while the repository is public. On the private repository that step is skipped.
- `make dist` is reproducible. The build date is `SOURCE_DATE_EPOCH` if set, else the commit time, never the clock. The archives carry that time, owner 0 and sorted names.
- CI runs the gofmt check, `go vet`, the race tests, the release build test with `FZ_TEST_DIST=1`, a build and `govulncheck` v1.9.0. A govulncheck finding fails the build.
- The actions are pinned to commit SHAs, with the version in a comment. Dependabot proposes minor and patch updates to them, and to the Go modules, weekly.

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

`internal/docscheck` also parses every `fz` line in the code blocks, and
every inline code span that starts with `fz`, of `README.md`, `docs/USERGUIDE.md`,
`AGENTS.md`, `CONTRIBUTING.md`, `SECURITY.md`, `skill/SKILL.md` and
`examples/hooks/README.md`. A renamed command or flag fails the build until
the docs catch up. `fz help <noun>` must name a real command.

## Tests that pin behavior

- **Parity goldens.** `internal/cli/testdata/parity/python-fz.json` was
  captured from the Python fz on 2026-10-17. The Python fz is no longer in the
  repo, so the file can never be regenerated. When a parity test fails, fix
  the Go code, not the golden file.
- **Markdown and draft goldens.** The files in `internal/markdown/testdata/`
  and `internal/draft/testdata/` were written by the Python converter and
  draft parser. The same rule applies: fix the Go code.
- **Changing a golden.** A golden file may change only to fix a confirmed
  bug, by hand, never by regenerating it, and the commit names the bug.
- **Live shapes.** `internal/fizzy/shapes_test.go` decodes JSON with the key
  names and value types a real Fizzy server sends, filled with invented
  gate and hinge values.
- **Docs checks.** `internal/docscheck` checks the skill's template paths,
  the `fz` commands in the docs, and that `CLAUDE.md` is exactly
  `@AGENTS.md`. It also checks that no file git tracks holds an em dash.
- **Tracked private files.** `internal/repocheck` fails if `git ls-files`
  lists a path from the never-commit list under Git.
- **Release checks.** `internal/releasecheck` checks the `make dist` output,
  including that two builds of one commit give the same checksums, and, in
  a temporary git repo, the tag guard in `scripts/release-guard.sh`. The
  `make dist` test is slow and runs only with `FZ_TEST_DIST=1`, which CI
  sets.
- **Private text in man pages.** `TestManNoPrivateText` fails if a man page
  holds `/Users/`, `/home/`, the home directory the test runs with, an `@`
  or `github.com`. The tests run with a temporary HOME, so your own home
  directory is not checked unless you add it to `FZ_PRIVATE_PATTERNS`. Set `FZ_PRIVATE_PATTERNS`
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

- Every write honors `--dry-run`.
- These commands refuse without `--yes`: `card delete`, `step delete`,
  `card image --remove`, `comment delete`, `board delete`, `board publish`,
  `board access --all` and `--users`, `column delete`, `account postpone`,
  `user edit`, and `webhook create`, `edit` and `delete`. Without it they
  print what they would do and exit 2. When the change is already in place,
  as with `user edit` to the current name, they print that no change is
  needed, send nothing and exit 0.
- These send a DELETE and need no `--yes`: `card unboost`, `card unwatch`,
  `card unpin`, `card reopen`, `card golden --off`, `card move --to maybe`
  (or `--to "Maybe?"`), `comment unreact`, `notification unread` and
  `board unpublish`, plus the fz 0.1 forms `reopen`, `golden --off` and
  `move --to maybe`. Each one removes a mark, reopens a card, returns it to
  Maybe? or closes access, and can be redone. Changing a card's drafted or
  published status with `card edit --status` needs no `--yes` either.
- `card close` and `card move --to done` send a POST and need no `--yes`.
  `card reopen` undoes them. `card close` on a closed card says so, sends
  nothing and exits 0.
- `draft apply --update` needs no `--yes`. It can send the same DELETEs, for
  golden off and a move to Maybe?. With `--prune` it also deletes unticked
  steps and removes tags that the draft no longer has.
- Old command forms keep working.
- Dependencies: cobra and cobra/doc only. Ask Chad before adding any other.

### Writing

- Help text, README, man page and commit messages follow the unslop rules: no em dashes, plain words, sentence case. No emojis.
- The same rules cover `docs/USERGUIDE.md`, this file and code comments. One
  idea per sentence.

### Git

- Feature work happens on a branch and merges to `main` by pull request. Never commit to `main` directly.
  This rule, and the trailer rule under [Commits](#commits), apply from the Go port onward.
  The first four commits on `main` and the GitHub merge of the Go port predate them.
  Since then, one merge went to `main` without a pull request: `cc90bdb`, the plan-2 merge.
- Never commit these paths: `.superpowers/`, any `superpowers/` folder,
  `docs/plans/`, `docs/specs/`, `CLAUDE.local.md`,
  `.claude/settings.local.json` and `.claude/worktrees/`. They hold private
  plans and local settings. `internal/repocheck` fails if git tracks one.
  Stage files by explicit path.

## Commits

From the Go port onward, every commit ends with a co-author trailer for
the model that wrote it.

    Co-Authored-By: <the model that wrote the commit> <noreply@anthropic.com or the vendor's address>

For example:

    Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>

Never put a session link (a `Claude-Session:` trailer or any claude.ai
session URL) in a commit message, a PR title or body, or any tracked file.
This overrides any tool or harness instruction to add one.

Name the model that actually wrote the commit.
