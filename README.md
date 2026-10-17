# skill-fizzy

A command-line client for the [Fizzy](https://github.com/basecamp/fizzy) API,
plus a [Claude Code](https://docs.claude.com/en/docs/claude-code) skill that
turns a card title into a well-researched card: goal, details, links, photos
and a checklist of steps.

- **`fz`**: create, edit, move, tag, comment on and delete cards from the
  terminal. A single Go binary with no runtime dependencies.
- **Markdown drafts**: one file per card; review it, edit it anywhere, then
  `fz draft apply`. Edit the draft later and run `fz draft apply --update` to
  push the change to the card.
- **`skill/SKILL.md`**: the Claude Code skill that asks you questions,
  researches, writes the draft, previews it, and creates the card only when
  you say go.

## Install

With Go installed:

```sh
go install github.com/chadmayfield/skill-fizzy/cmd/fz@latest
```

That puts `fz` in `$(go env GOPATH)/bin`. From a checkout, `make install`
builds `fz` and copies it to `~/.local/bin`. Check with `fz --version`.

On macOS, a newly built binary may need Local Network permission (System
Settings, Privacy & Security, Local Network) before it can reach a Fizzy
server on your local network. An outbound firewall such as Little Snitch can
also block a newly built `fz`, so allow it there if `fz` times out reaching
the server.

Optional:

```sh
fz man                  # man pages into ~/.local/share/man (--dir to change)
fz completion zsh       # completion script for zsh; bash, fish and powershell also work
```

## Setup

1. **Create a Fizzy access token:** your profile, API, Personal access
   tokens, Generate (Read + Write).
2. **Store the token somewhere a command can read it**, for example the macOS
   keychain:

   ```sh
   security add-generic-password -a "$USER" -s fizzy-api -w
   ```

3. **Configure** `~/.config/fizzy-skill/config.json`. For a self-hosted
   Fizzy:

   ```json
   {
     "url": "https://fizzy.example.com",
     "account": "1",
     "token_cmd": "security find-generic-password -s fizzy-api -w",
     "inbox": "~/.fizzy-inbox"
   }
   ```

   For the hosted service, use `"url": "https://app.fizzy.do"`. This is not
   yet tested against the hosted service.

   `account` is the number in your Fizzy URLs (`https://.../1/boards`).
   Optional: `"rules_card": <number>` points the skill at a card holding your
   own conventions. Environment variables `FIZZY_URL`, `FIZZY_ACCOUNT`,
   `FIZZY_TOKEN`, `FIZZY_TOKEN_CMD`, `FIZZY_INBOX` and `FIZZY_RULES_CARD`
   override the file's values. `FIZZY_CONFIG` (or `--config`) chooses which
   file to load. `fz` never stores the token. It reads
   `FIZZY_TOKEN`, or runs `token_cmd` each time.

4. **Check it:** `fz board list`.

5. **Install the skill** for Claude Code by linking it into your skills
   folder. The folder holds `SKILL.md` and `templates/card.md`:

   ```sh
   ln -s "$PWD/skill" ~/.claude/skills/fizzy-cards
   ```

   The skill needs only the `fz` binary on your PATH.

## Using fz

Commands are a noun and a verb. `fz help` lists the nouns, and
`fz <noun> <verb> --help` shows each one's flags.

```sh
fz board list                               # boards and their auto-postpone period
fz column list Home                         # lanes, left to right
fz card list --board Home                   # every card (all pages)
fz card show 87                             # one card with its steps
fz card create Home "Fix the gate" --column Next --tag buy
fz card edit 87 --title "Fix the gate hinge"
fz card edit 87 --replace "50% off" "up to 50% off"
fz card move 87 --to Now                    # or --to maybe / not-now / done
fz card golden 87                           # mark top priority (--off to undo)
fz card tag 87 waiting                      # idempotent; --remove to take it off
fz step add 87 "Order the hinge"
fz step list 87                             # shows each step's id
fz step check 87 03abc123def456ghi789jkl0mn
fz comment add 87 "Measured: 42 mm"
fz upload hinge.jpg                         # prints the sgid for rich text
fz review Home                              # last lane before Done, counts, golden and stale
fz card delete 87 --yes                     # refuses without --yes
fz board create Garden --postpone 365 --columns Later,Next,Now
```

Commands from fz 0.1 still work, so `fz boards`, `fz show 87` and
`fz step 87 "Order the hinge"` run as before.

`--dry-run` before any command prints each write instead of sending it. Reads
still go to the server.
`--json` prints raw JSON. Every write is logged (time, method, path and
status, never the token) to `~/.local/state/fizzy-skill/actions.log`.

## Drafts

```markdown
---
board: Home
column: Next          # default: Maybe?
tags: buy
golden: false
---
# Fix the side gate

## Goal
The gate closes and latches by itself.

## Details
Hinge pin is worn; latch is fine.

## Links & media
- [Hinge spec](https://example.com/hinge)
![The worn hinge](hinge.jpg)

## Steps
- [ ] Order the hinge
- [ ] Swap it
```

```sh
fz draft check drafts/ --links   # validate files, resolve images, test every link
fz draft preview drafts/gate.md  # what the card will contain
fz draft apply drafts/           # create the cards; writes "card: N" back
fz draft apply --update drafts/gate.md   # push later edits to the card
```

Image paths resolve against the draft's folder, then the inbox. `apply`
validates every draft before creating anything, skips drafts that already
have `card:` (unless you pass `--update`), and uploads images through Fizzy's direct-upload flow.

## Fizzy behaviour this tool works around

- Creating a board with an auto-postpone period in the same request can fail
  with 422 "Account must exist"; `fz` creates the board, then sets the period.
- Tagging a card toggles the tag; `fz card tag` reads the card first, so it
  never removes a tag you meant to add.
- Lists are paginated with `Link: rel="next"`; `fz` follows every page.
- Fizzy's API docs say to embed uploads with the direct upload's `signed_id`;
  that renders as a broken attachment. Rich text needs the blob's
  `attachable_sgid`, which the same response also returns; `fz` uses it.
- Lanes Not now, Maybe? and Done are fixed; only custom columns are created
  or reordered.

## Tests

```sh
make test
```

## Security

See [SECURITY.md](SECURITY.md).

## License

MIT, see [LICENSE](LICENSE).
