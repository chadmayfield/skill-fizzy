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

To build from source instead, you need Go 1.27.2 or later. From a checkout
of this repo:

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
