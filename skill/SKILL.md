---
name: fizzy-cards
description: Build well-researched Fizzy cards with the user. Use when the user gives a card title (or a list of titles) for a Fizzy board, asks to add, draft, refine, move, or organize Fizzy cards, or hands over a checklist to turn into cards. Drafts Markdown files, previews them in chat, and creates cards with the fz CLI only after the user approves.
---

# Fizzy cards

You help the user turn a title (or a curated list) into Fizzy cards that hold
everything needed to act: a goal, details, links and media, and steps. You
work through the `fz` command-line client in this repository.

## Before you start

1. Check `fz` works: `fz boards`. If it fails with a config error, point the
   user at the README's setup section and stop.
2. If a rules card is configured (`rules_card` in the config), read it with
   `fz show <number>` and follow it. It overrides anything below.

## Golden rules

- **Nothing is created, changed, moved or deleted in Fizzy until the user says
  go** for that specific draft or batch. Previews are free; writes are not.
- **Never delete** a card, board, column or tag without explicit approval,
  and use `fz delete <n> --yes` only after showing what will be deleted.
- **Never print or echo the API token.** `fz` reads it itself.
- Use `--dry-run` whenever you are unsure what a command will send.

## One card

1. **Get a title and a board.** Board names come from `fz boards`.
2. **Ask questions one at a time** (3 to 5 total, fewer if the answers are
   obvious). Choose questions for this card, for example:
   - What does "done" look like?
   - Constraints: budget, time, tools, safety, must it be reversible?
   - What is already known or decided?
   - Any deadline or season?
   - Photos or screenshots to attach? (They go in `<inbox>/<card-slug>/`.)
   - **How deep should research go for this card:** quick (a few key links)
     or deep (compare options)? Ask this every card.
3. **Research** to the depth chosen. Prefer primary sources: manufacturer
   pages, manuals, official docs, the user's own repos and sites. Link to
   images on the web rather than copying them; embed only the user's own
   files.
4. **Write the draft** to `<inbox>/drafts/<card-slug>.md` using
   `templates/card.md`: front matter (board, column, tags, golden), then
   `# Title`, `## Goal`, `## Details`, `## Links & media`, `## Steps`.
   - Column defaults to **Maybe?** unless the user names one.
   - Steps are concrete actions, `- [ ] ...`.
   - Big projects get fewer, bigger cards with the work in their steps.
5. **Check it:** `fz draft check <file> --links`. Fix or remove every link
   that does not return OK. Dead links are bugs.
6. **Preview in chat:** show the board, column, tags, the Goal / Details /
   Links & media text and the steps, in plain Markdown (not raw HTML).
7. **On the user's go:** `fz draft apply <file>`, then give the card link.
   `apply` writes `card: <number>` back into the draft so it is never created
   twice.

## A batch (a curated list)

1. Ask once which board the list belongs to and any shared context.
2. Draft **all** cards into `<inbox>/drafts/<batch-name>/`, asking only the
   questions you cannot answer from the list itself.
3. `fz draft check <folder> --links`, then preview the whole batch in chat as
   a compact list (title, column, step count), with full text on request.
4. On go: `fz draft apply <folder>`. Report each card's number and link.

## Other operations

| Want | Command |
|---|---|
| List cards | `fz cards --board "<board>" [--state maybe/closed/not_now/golden]` |
| Show a card | `fz show <n>` |
| Move | `fz move <n> --to "<column>"`, `--to maybe`, `--to not-now`, `--to done`; `--board "<board>"` to change board |
| Reopen | `fz reopen <n>` |
| Golden | `fz golden <n>` / `fz golden <n> --off` |
| Tag | `fz tag <n> <tag>` / `--remove` (safe: checks before toggling) |
| Step | `fz step <n> "text"`, `fz step <n> --check <step-id>` |
| Comment | `fz comment <n> "Markdown text"` |
| Delete | `fz delete <n> --yes` (only with approval) |

## Fizzy facts worth knowing

- Lanes are fixed left to right: **Not now → Maybe? → (your columns) → Done**.
  Only custom columns can be added or reordered.
- Cards nobody touches move to Not now after the board's auto-postpone period
  (3, 7, 11, 30, 90 or 365 days).
- Tags are created by tagging a card; there is no empty tag.
- Lists are paginated; `fz` always reads every page.
