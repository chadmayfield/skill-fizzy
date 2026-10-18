# Contributing

Thanks for helping with `fz` and the fizzy-cards skill. This page covers the
basics. [AGENTS.md](AGENTS.md) holds the full rules.

## Prerequisites

- Go 1.27.2 or later. `go.mod` sets the version, and CI reads it from there.
- `actionlint`, if you change a file in `.github/workflows/`.

## Build and test

```sh
make build
make test
go vet ./...
gofmt -l .
```

`make build` writes `./fz`. `make test` runs `go test -race -count=1 ./...`.
`gofmt -l .` must print nothing. Run `actionlint` after a workflow edit.

## Branches and pull requests

Work on a branch and open a pull request into `main`. Nobody commits to
`main` directly. Stage files by name, not with `git add .`.

## Adding a command

Each command lives in its own file in `internal/cli/`. The file registers
the command by calling `register()` from its `init()`, so no central list
needs an edit. Give the command a `Short`, a `Long` and an `Example`, and add
it to `wantCommands` in `internal/cli/help_test.go`. When a command changes,
update `docs/USERGUIDE.md` in the same commit.

## Tests that must not be regenerated

`internal/cli/testdata/parity/python-fz.json` was captured from the Python
fz, which is no longer in the repo. It can never be regenerated. When a
parity test fails, fix the Go code, not the golden file.

## Private files and test data

`internal/repocheck` fails the build if git tracks a planning or local
settings file, such as anything under `.superpowers/`, `docs/plans/` or
`docs/specs/`.

Tests, examples and docs use neutral data only: card 87, step id
`03abc123def456ghi789jkl0mn`, the gate and hinge theme, and
`fizzy.example.com`. Never use a real server URL, account, token, name or
card title.

## Releases

A release is a SemVer tag, such as `v1.2.0` or `v1.2.0-rc.1`, pushed on a
commit that is already on `main`. The release workflow checks the tag with
`scripts/release-guard.sh`, runs the tests, builds the archives and
publishes the release. A tag with a suffix becomes a prerelease.

## Security

Report a security problem through GitHub's private vulnerability reporting,
not a public issue. See [SECURITY.md](SECURITY.md).
