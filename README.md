# skill-fizzy

`fz` is a Go command-line client for the [Fizzy](https://github.com/basecamp/fizzy)
API. This repo also holds a Claude Code skill that uses `fz` to turn a card
title into a researched card with a goal, details, links, photos and steps.
The skill writes a Markdown draft, shows it to you, and creates the card only
when you say so.

## Install

Each release on the [releases page](https://github.com/chadmayfield/skill-fizzy/releases)
holds one archive per system, with `checksums.txt`. An archive holds the
`fz` binary, this README, the license and the user guide. Put `fz` on your
PATH, for example in `~/.local/bin`. The user guide's Install section shows
how to check the archive first.

With Go 1.27.3 or later you can install it directly:

```sh
go install github.com/chadmayfield/skill-fizzy/cmd/fz@latest
```

That puts `fz` in `$(go env GOPATH)/bin`. To build from a checkout of this
repo instead:

```sh
make install
```

That builds `fz` and copies it to `~/.local/bin`, creating the folder if it
is missing. Put `~/.local/bin` on your PATH if it is not there already.

## Configure

Put this in `~/.config/fizzy-skill/config.json`, with your server, your
account number and a command that prints your access token:

```json
{"url": "https://fizzy.example.com", "account": "1",
 "token_cmd": "security find-generic-password -s fizzy-api -w"}
```

Then check that `fz` can reach the server with `fz board list`.

## Examples

```sh
fz card list --board Home
fz card create Home "Fix the gate" --column Next --tag buy
fz card assign 87 "Gate Keeper"
fz step add 87 "Order the hinge"
fz card move 87 --to done
fz draft apply --update gate.md
```

Add `--dry-run` before any command to print its writes instead of sending
them. `fz help` lists the commands.

## Compatibility

Fizzy's API has no version number, so this table names Fizzy commits.

| fz | Fizzy API docs | Tested against |
|---|---|---|
| v0.1.1 | [`7e4b5e5`](https://github.com/basecamp/fizzy/commit/7e4b5e5532819df8bb59a11502ed2e36f3eb9160) (2026-09-10) | Fizzy built from [`663a92d`](https://github.com/basecamp/fizzy/commit/663a92d05f4486d74ce69d5dba8267fa19593c08) (2026-10-14) |
| v0.1.0 | [`7e4b5e5`](https://github.com/basecamp/fizzy/commit/7e4b5e5532819df8bb59a11502ed2e36f3eb9160) (2026-09-10) | Fizzy built from [`663a92d`](https://github.com/basecamp/fizzy/commit/663a92d05f4486d74ce69d5dba8267fa19593c08) (2026-10-14) |

The API docs column is the last change to Fizzy's `docs/api` that `fz`
follows. A newer Fizzy usually works. If Fizzy's API docs change after that
commit, check this table and the release notes.

## More

These two come with every release archive:

- [User guide](docs/USERGUIDE.md): setup, every command, drafts, the skill
  and the session hook.
- [License](LICENSE): MIT.

These are in the [repo](https://github.com/chadmayfield/skill-fizzy):

- [Security](https://github.com/chadmayfield/skill-fizzy/blob/main/SECURITY.md):
  how `fz` handles your token, and how to report a problem.
- [AGENTS.md](https://github.com/chadmayfield/skill-fizzy/blob/main/AGENTS.md):
  how to build, test and change this repo, for people and coding agents.
- [Contributing](https://github.com/chadmayfield/skill-fizzy/blob/main/CONTRIBUTING.md):
  build, test and pull requests, for human contributors.
