# skill-fizzy

A small command-line client for the [Fizzy](https://github.com/basecamp/fizzy)
API, plus a [Claude Code](https://docs.claude.com/en/docs/claude-code) skill
that turns a card title into a well-researched card: goal, details, links,
photos and a checklist of steps.

- **`fz`**: create, move, tag, comment on and delete cards from the terminal.
  Python 3.9+, standard library only, no dependencies.
- **Markdown drafts**: one file per card; review it, edit it anywhere, then
  `fz draft apply`.
- **`skill/SKILL.md`**: the Claude Code skill that asks you questions,
  researches, writes the draft, previews it, and creates the card only when
  you say go.

## Setup

1. **Create a Fizzy access token:** your profile → API → Personal access
   tokens → Generate (Read + Write).
2. **Store the token somewhere a command can read it**, for example the macOS
   keychain:

   ```sh
   security add-generic-password -a "$USER" -s fizzy-api -w
   ```

3. **Configure** `~/.config/fizzy-skill/config.json`:

   ```json
   {
     "url": "https://fizzy.example.com",
     "account": "1",
     "token_cmd": "security find-generic-password -s fizzy-api -w",
     "inbox": "~/.fizzy-inbox"
   }
   ```

   `account` is the number in your Fizzy URLs (`https://.../1/boards`).
   Optional: `"rules_card": <number>` points the skill at a card holding your
   own conventions. Environment variables `FIZZY_URL`, `FIZZY_ACCOUNT`,
   `FIZZY_TOKEN`, `FIZZY_TOKEN_CMD`, `FIZZY_INBOX`, `FIZZY_RULES_CARD`
   override the file.

4. **Run it** from the checkout (`bin/fz`) or install it (`pip install .`, which
   adds `fz` to your PATH). Check with `fz boards`.

5. **Install the skill** for Claude Code by linking it into your skills folder:

   ```sh
   ln -s "$PWD/skill" ~/.claude/skills/fizzy-cards
   ```

## Using fz

```sh
fz boards                                   # boards and their auto-postpone period
fz columns Home                             # lanes, left to right
fz cards --board Home                       # every card (all pages)
fz show 87                                  # one card with its steps
fz create Home "Fix the gate" --column Next --tag buy
fz move 87 --to Now                         # or --to maybe / not-now / done
fz golden 87                                # mark top priority (--off to undo)
fz tag 87 waiting                           # idempotent; --remove to take it off
fz step 87 "Order the hinge"
fz comment 87 "Measured: 42 mm"
fz delete 87 --yes                          # refuses without --yes
fz board-create Garden --postpone 365 --columns Later,Next,Now
```

`--dry-run` before any command prints what would be sent, without sending it.
`--json` prints raw JSON. Every write is logged (method, path, status, never
the token) to `~/.local/state/fizzy-skill/actions.log`.

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
```

Image paths resolve against the draft's folder, then the inbox. `apply`
validates every draft before creating anything, skips drafts that already
have `card:`, and uploads images through Fizzy's direct-upload flow.

## Fizzy behaviour this tool works around

- Creating a board with an auto-postpone period in the same request can fail
  with 422 "Account must exist"; `fz` creates the board, then sets the period.
- Tagging a card toggles the tag; `fz tag` checks first, so it never removes a
  tag you meant to add.
- Lists are paginated with `Link: rel="next"`; `fz` follows every page.
- Fizzy's API docs say to embed uploads with the direct upload's `signed_id`;
  that renders as a broken attachment. Rich text needs the blob's
  `attachable_sgid`, which the same response also returns; `fz` uses it.
- Lanes Not now, Maybe? and Done are fixed; only custom columns are created
  or reordered.

## Tests

```sh
python3 -m unittest discover -s tests
```

## License

MIT, see [LICENSE](LICENSE).
