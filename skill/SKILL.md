---
name: fizzy-cards
description: Build well-researched Fizzy cards with the user. Use when the user gives a card title (or a list of titles) for a Fizzy board, asks to add, draft, refine, move, or organize Fizzy cards, or hands over a checklist to turn into cards. Drafts Markdown files, previews them in chat, and creates cards with the fz CLI only after the user approves.
---

# Fizzy cards

You help the user turn a title (or a curated list) into Fizzy cards that hold
everything needed to act: a goal, details, links and media, and steps. You
work through `fz`, a command-line client for the Fizzy API. `fz` is a single
Go binary and must be on the PATH.

## Before you start

1. Check `fz` is installed: `fz --version`. If the command is not found, tell
   the user to install it (the README's install section) and stop.
2. Check it can reach Fizzy: `fz board list`. If it fails with a config error,
   point the user at the README's setup section and stop.
3. If a rules card is configured (`rules_card` in the config), read it with
   `fz card show <number>` and follow it. It overrides anything below.

## Golden rules

- **Nothing is created, changed, moved or deleted in Fizzy until the user says
  go** for that specific draft or batch. Previews are free; writes are not.
- **Never delete** a card, board, column or tag without explicit approval,
  and use `fz card delete <n> --yes` only after showing what will be deleted.
- **Never print or echo the API token.** `fz` reads it itself.
- Use `--dry-run` whenever you are unsure what a command will send.

## One card

1. **Get a title and a board.** Board names come from `fz board list`.
2. **Ask questions one at a time** (3 to 5 total, fewer if the answers are
   obvious). Choose questions for this card, for example:
   - What does "done" look like?
   - Constraints: budget, time, tools, safety, must it be reversible?
   - What is already known or decided?
   - Any deadline or season?
   - Photos or screenshots to attach? (Put the file next to the draft, or
     under the inbox and write the path relative to it, such as
     `<card-slug>/file.jpg`.)
   - **How deep should research go for this card:** quick (a few key links)
     or deep (compare options)? Ask this every card.
3. **Research** to the depth chosen. Prefer primary sources: manufacturer
   pages, manuals, official docs, the user's own repos and sites. Link to
   images on the web rather than copying them; embed only the user's own
   files.
4. **Write the draft** to `<inbox>/drafts/<card-slug>.md` using
   `templates/card.md` (in this skill's folder): front matter (board, column, tags, golden), then
   `# Title`, `## Goal`, `## Details`, `## Links & media`, `## Steps`.
   - Column defaults to **Maybe?** unless the user names one.
   - Steps are concrete actions, `- [ ] ...`.
   - Put each image on a line by itself, `![alt](file.jpg)`, not in a list
     item. Otherwise it shows as text instead of a photo.
   - Big projects get fewer, bigger cards with the work in their steps.
5. **Check it:** `fz draft check <file> --links`. Fix or remove every link
   that does not return OK. Dead links are bugs.
6. **Preview in chat:** show the board, column, tags, the Goal / Details /
   Links & media text and the steps, in plain Markdown (not raw HTML).
7. **On the user's go:** `fz draft apply <file>`, then give the card link.
   `apply` writes `card: <number>` back into the draft so it is never created
   twice.
8. **Changing the card later:** edit the draft, then run
   `fz draft apply --update <file>`. It pushes the draft to the card and
   refuses a card that changed in Fizzy since the last sync unless you add
   `--force`. It never unticks a step. `--prune` also deletes unticked steps and tags
   the draft no longer has, so use it only with approval. Run
   `fz --dry-run draft apply --update <file>` first.

## A batch (a curated list)

1. Ask once which board the list belongs to and any shared context.
2. Draft **all** cards into `<inbox>/drafts/<batch-name>/`, asking only the
   questions you cannot answer from the list itself.
3. `fz draft check <folder> --links`, then preview the whole batch in chat as
   a compact list (title, column, step count), with full text on request.
4. On go: `fz draft apply <folder>`. Report each card's number and link.

## While you work on a mapped repo

A repo can map to a board, or to one card on it. When the session's work
matches a card:

- When you finish one of its steps, offer once to tick it:
  `fz step check <n> <step-id>`. Get the id from `fz step list <n>`.
- When the card's goal is met, ask before moving it to Done.
- Never create, move, tick or delete anything without the user's OK.

To see the board's Now and Next cards at the start of every session, run
`fz hook session-start` from a Claude Code SessionStart hook. Setup is in
`examples/hooks/README.md` in the fz repo.

## Other operations

Commands are nouns followed by a verb. Run `fz help <noun>` or
`fz <noun> <verb> --help` for flags. Commands from fz 0.1 (`fz boards`,
`fz show 87`, `fz step 87 "text"`) still work.

| Want | Command |
|---|---|
| List boards | `fz board list` |
| List lanes | `fz column list <board>` |
| List cards | `fz card list --board "<board>" [--state maybe/closed/not_now/golden] [--search "<terms>"]` |
| Show a card | `fz card show <n>` |
| Create a card | `fz card create "<board>" "<title>" [--column "<column>"] [--tag <tag>]` |
| Move | `fz card move <n> --to "<column>"`, `--to maybe`, `--to not-now`, `--to done`; `--board "<board>"` to change board |
| Reopen | `fz card reopen <n>` |
| Golden | `fz card golden <n>` / `fz card golden <n> --off` |
| Tag | `fz card tag <n> <tag>` / `--remove` (safe: checks before toggling) |
| Retitle or redescribe | `fz card edit <n> --title "<title>"` or `--description-file <file>` |
| Fix one phrase | `fz card edit <n> --replace "<old>" "<new>"` (add `--all` for every match) |
| List steps | `fz step list <n>` (shows each step id) |
| Add a step | `fz step add <n> "text"` |
| Tick a step | `fz step check <n> <step-id>` / `fz step uncheck <n> <step-id>` |
| Edit a step | `fz step edit <n> <step-id> "text"` |
| Delete a step | `fz step delete <n> <step-id> --yes` (only with approval) |
| Comment | `fz comment add <n> "Markdown text"` |
| Upload a file | `fz upload <file>` prints the sgid for rich text |
| Weekly look | `fz review [<board>...]` lists the last column before Done (whatever its name), counts the other columns and Maybe?, and lists golden, postponing-soon and stalled cards, per board |
| Delete | `fz card delete <n> --yes` (only with approval) |

A new description from `fz card edit` replaces the whole description,
including embedded images. For a small fix, use `--replace`.

## Fizzy facts worth knowing

- Lanes are fixed left to right: **Not now → Maybe? → (your columns) → Done**.
  Only custom columns can be added or reordered.
- Cards nobody touches move to Not now after the board's auto-postpone period
  (3, 7, 11, 30, 90 or 365 days).
- Tags are created by tagging a card; there is no empty tag.
- Lists are paginated; `fz` always reads every page.
