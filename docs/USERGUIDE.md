# fz user guide

`fz` is a command-line client for the [Fizzy](https://github.com/basecamp/fizzy)
API. It lists, creates, edits, moves, tags and deletes cards, and works on
their steps, comments, assignees and boosts. It also manages boards, columns,
notifications, exports and webhooks. It turns Markdown drafts into cards and
pushes later draft edits back to them.

The repo also holds the fizzy-cards skill for Claude Code. The skill asks you
about a card, researches it, writes a draft and creates the card through `fz`
once you approve it.

This guide is for anyone who keeps work in Fizzy and wants to manage it from
a terminal or from Claude Code.

## Contents

- [Before you start](#before-you-start)
- [Setup](#setup)
- [Using fz](#using-fz)
- [Drafts](#drafts)
- [The skill](#the-skill)
- [The session hook](#the-session-hook)
- [Troubleshooting](#troubleshooting)

## Before you start

You need these:

- Go 1.27.2 or later, to build `fz`.
- A Fizzy server. A self-hosted server works. The hosted Fizzy service has not
  been tested with `fz`.
- A Fizzy personal access token that can read and write.

On macOS, a newly built `fz` may need Local Network permission before it can
reach a server on your local network. Grant it in System Settings, under
Privacy & Security, Local Network. An outbound firewall such as Little Snitch
can also block a newly built binary, so allow `fz` there too.

## Setup

### Install

There is no release yet, so build `fz` from source. From a checkout of this
repo:

```sh
make install
```

That builds `fz` and copies it to `~/.local/bin`, creating the folder if it
is missing. Check the install with `fz --version`.

### Install from a release

Once a release is published, it holds one archive per system (macOS or
Linux, arm64 or amd64) and `checksums.txt`. Each archive is named after the
release tag, `v` included, such as `fz_v1.2.0_linux_amd64.tar.gz`. Download
yours and `checksums.txt`. In the folder that holds them, check the archive
against the checksums. On Linux:

```sh
sha256sum -c checksums.txt --ignore-missing
```

On macOS, or wherever `sha256sum` is missing:

```sh
shasum -a 256 -c checksums.txt --ignore-missing
```

A release made while the repo is public also carries a Sigstore
build-provenance attestation for each archive. To check that GitHub Actions
built it from this repo:

```sh
gh attestation verify fz_v<version>_<os>_<arch>.tar.gz -R chadmayfield/skill-fizzy
```

Unpack the archive and put the `fz` binary on your PATH, for example:

```sh
tar -xzf fz_v<version>_<os>_<arch>.tar.gz
mkdir -p ~/.local/bin
install fz_v<version>_<os>_<arch>/fz ~/.local/bin/fz
```

macOS may block a downloaded binary the first time you run it. Remove the
quarantine flag with `xattr -d com.apple.quarantine ~/.local/bin/fz`, or allow
it in System Settings, Privacy & Security.

### Store the token

`fz` never stores the token. It takes the token from `FIZZY_TOKEN`, or runs a
token command each time it needs one and uses what the command prints.

Keep the token in a keychain or another secret store. On macOS, this command
prompts for the token and saves it in the login keychain:

```sh
security add-generic-password -a "$USER" -s fizzy-api -w
```

The matching token command is
`security find-generic-password -s fizzy-api -w`.

### The config file

`fz` reads `~/.config/fizzy-skill/config.json`:

```json
{
  "url": "https://fizzy.example.com",
  "account": "1",
  "token_cmd": "security find-generic-password -s fizzy-api -w",
  "inbox": "~/.fizzy-inbox",
  "rules_card": 87
}
```

| Key | What it holds |
|---|---|
| `url` | The server's address, such as `https://fizzy.example.com`. Required. It must use https. Plain http is allowed only for localhost, 127.0.0.1 and ::1. It cannot hold a user name, a password, a query or a fragment. |
| `account` | The account number, the `1` in `https://fizzy.example.com/1/boards`. Required. |
| `token_cmd` | A command that prints the token. `fz` splits it into words like a shell does, but does not run it through a shell. |
| `inbox` | The inbox folder. The default is `~/.fizzy-inbox`. |
| `rules_card` | The number of a card that holds your own conventions for the skill. Optional. |

A missing config file is not an error, as long as the environment supplies
the URL and account.

The file names a command that `fz` runs, so `fz` refuses a config file that
the group or others can write. `chmod 600` on the file fixes that. The file
cannot hold the token itself, so a `token` key is an error.

### Environment variables

Each variable overrides the matching key in the file.

| Variable | Overrides |
|---|---|
| `FIZZY_URL` | `url` |
| `FIZZY_ACCOUNT` | `account` |
| `FIZZY_TOKEN_CMD` | `token_cmd` |
| `FIZZY_INBOX` | `inbox` |
| `FIZZY_RULES_CARD` | `rules_card` |
| `FIZZY_TOKEN` | Has no key in the file. When set, `fz` uses it and runs no token command. |

`FIZZY_CONFIG` names a different config file to load. The `--config` flag
does the same for one command and wins over `FIZZY_CONFIG`.

### The inbox

The inbox is a folder for drafts and the images they use. The skill writes
drafts to `<inbox>/drafts/`. When a draft names an image by a relative path,
`fz` looks next to the draft first, then in the inbox.

### The rules card

`rules_card` points the skill at one card on your server. Its description
holds your own rules, such as which columns to use or what every card needs.
The skill reads it with `fz card show` before it starts and follows it over
its own defaults. The card can add rules, but it can never loosen the
skill's golden rules, such as asking before every change and never deleting
without approval. `fz` itself only reads the number from the config.

### Check it

```sh
fz board list
```

This prints each board's id, name and auto-postpone period. If it fails, see
[Troubleshooting](#troubleshooting).

## Using fz

Commands are a noun and then a verb, such as `fz card show 87`. Run
`fz help` for the nouns and `fz help card` for the verbs under one noun.
`fz card edit --help` shows one command's flags and examples.

### Flags for every command

| Flag | Effect |
|---|---|
| `--dry-run` | Prints each write instead of sending it. Reads still go to the server, so a dry run shows real ids and names. |
| `--json` | Prints JSON instead of text. |
| `--config FILE` | Loads this config file. |

```sh
fz --dry-run card move 87 --to done
fz --json card show 87
```

In a dry run, the commands added after fz 0.1 end with a line such as
`dry-run: nothing sent; would watch card #87`. When the change is already in
place, such as assigning a user who is assigned, the line is
`dry-run: nothing sent; no change needed`. With `--json` they add
`"dry_run": true` to their output. The fz 0.1 commands print the same text
and JSON as the Python fz did.

### Exit status

| Code | Meaning |
|---|---|
| 0 | Success. |
| 1 | An API, config or draft error. The message says which. |
| 2 | A usage error, such as a bad flag or an unknown command, or a refusal. fz refuses a delete without `--yes`, a name that two users, boards, columns or tags share, and a change it cannot check first. It also refuses an id that is empty, `.` or `..`, a USER that matches nobody, and an empty or blank name, title, step, comment, boost or reaction, and sends nothing. |

### Boards

```sh
fz board list
fz board create Garden --postpone 365 --columns Later,Next,Now
```

`--postpone` is the number of days without activity before Fizzy moves a card
to Not now. `--columns` takes custom columns, left to right. `fz` creates the
board first and then sets the period, because Fizzy can refuse both in one
request. The period and the columns need the new board's id from Fizzy's
reply. If the reply has none, `fz` says the board was created without them
and exits 1.

A board is named by its name or its id in every board command. Case does not matter. A name that two boards share in any case exits 2; use the board id.

Show and edit:

```sh
fz board show Home
fz board edit Home --name "Gate and hinge"
fz board edit Home --postpone 90
fz board edit Home --public-description "Notes on the gate"
```

`board show` prints the id, name, auto-postpone period, who can access the
board, who created it and its public link. The period is one of 3, 7, 11, 30,
90 or 365 days. The public description is Markdown. It shows on the public
page once the board is published.

Publish:

```sh
fz board publish Home --yes
fz board unpublish Home
```

Anyone with the public link can read a published board without logging in.
`board publish` prints the link and refuses without `--yes`. `board unpublish`
needs no `--yes`, because it only closes access.

Access:

```sh
fz board access Home
fz board access Home --all --yes
fz board access Home --users 03abc123def456ghi789jkl0mn --yes
fz board access Home --users Alex,Sam --yes
```

With no flag, `board access` lists who can reach the board. `--all` opens it
to every user in the account. `--users` takes a comma-separated list and
replaces the whole list, so anyone not named loses access. Each entry is a
user id, a name or an email, matched the way `card assign` matches a user.
If an entry matches nobody or more than one user, `fz` exits 2 and changes
nothing. Both changes print the old and new access first, with each user's
name and id, and refuse without `--yes`. The `--users` preview lists who
keeps access, who gains it and who loses it. When Fizzy's reply does not say
who has access now, the preview says the current list is unknown. `--all` on a board that every user
can already reach prints that no change is needed, exits 0 and sends
nothing. So does `--users` with the board's current list, in any order.
Neither needs `--yes` then.

Delete:

```sh
fz board delete Home --yes
```

This deletes the board and its cards for good. `fz` first prints how many
cards the board holds under Fizzy's `all`, `closed` and `not_now` card
filters, and its custom columns. Fizzy's API docs do not say which cards
`all` holds, so the counts may overlap.
Without `--yes` it deletes nothing and exits with status 2.

### Columns

```sh
fz column list Home
fz column show Home Next
fz column cards Home Next
```

`column list` prints a board's lanes from Not now to Done. Not now, Maybe?
and Done are fixed. Custom columns sit between Maybe? and Done. `column show`
prints one column's id, name and color. `column cards` prints one line for
each open card in the column. A column is named by its name or its id.

Create, edit and delete custom columns:

```sh
fz column create Home "Gate repairs" --color Lime
fz column edit Home Next --name "Hinge repairs"
fz column edit Home Next --color tan
fz column delete Home Next --yes
```

A new column goes to the right of the others. `--color` takes one of the
nine color names in Fizzy's API docs, in any case: Blue, Gray, Tan, Yellow,
Lime, Aqua, Violet, Purple and Pink. It also takes `default` for Blue, or a
number from 1 for Gray to 8 for Pink, so `--color 4` is Lime. It also takes
the exact value Fizzy stores for one, from `var(--color-card-default)` for
Blue to `var(--color-card-8)` for Pink. Any other color exits 2, lists the
accepted forms and sends nothing, because the server accepts an unknown color
without using it and still reports success. `column delete` prints the column and how many open
cards it holds, and refuses without `--yes`.

### Cards

A card is named by its number, the `87` in `/cards/87`.

List and show:

```sh
fz card list --board Home
fz card list --state golden
fz card list --search "gate hinge"
fz card show 87
```

`card list` reads every page Fizzy returns. `--state` passes Fizzy's index
filter, such as `maybe`, `closed`, `not_now` or `golden`. `card show` prints
the title, board, link, description and steps.

More filters narrow the list. They combine with AND.

```sh
fz card list --board Home --column Next --tag waiting
fz card list --assignee "Gate Keeper"
fz card list --unassigned --created thisweek --sort oldest
fz card list --closed lastmonth
```

| Flag | Keeps |
|---|---|
| `--column` | Cards in this column on `--board`. It needs `--board`. |
| `--tag` | Cards with this tag. |
| `--assignee` | Cards assigned to this user. See [Assign](#assign) for how fz matches a user. |
| `--unassigned` | Cards with no assignee. It cannot go with `--assignee`. |
| `--created`, `--closed` | Cards created or closed in a window: `today`, `yesterday`, `thisweek`, `lastweek`, `thismonth`, `lastmonth`, `thisyear` or `lastyear`. |
| `--sort` | Not a filter. It orders the list `latest` (the default), `newest` or `oldest`. |

Create:

```sh
fz card create Home "Fix the gate"
fz card create Home "Fix the gate" --column Next --tag buy --tag gate
fz card create Home "Fix the gate" --description-file gate.md
```

A new card lands in Maybe? unless `--column` names another lane. A
description is Markdown, given with `--description` or read from a file with
`--description-file`.

Edit:

```sh
fz card edit 87 --title "Fix the gate hinge"
fz card edit 87 --description-file gate.md
fz card edit 87 --replace "two hinges" "three hinges"
fz card edit 87 --replace hinge hinges --all
fz card edit 87 --description "" --allow-empty
fz card edit 87 --status published
```

- A new description replaces the whole description, including any embedded
  images.
- `fz` refuses a new description that is empty or only whitespace, and a
  `--replace` that leaves the description empty. Add `--allow-empty` to clear
  a description on purpose.
- `--replace OLD NEW` swaps one phrase in the current description and leaves
  the rest alone. `--all` swaps every match. Use it for small fixes, so
  images survive.
- `--replace` matches only the text you can read. It never matches a tag
  name or an attribute, such as a link's address, and a match cannot cross
  formatting. OLD must not be empty.
- `--status` takes `drafted` or `published`.

Move, close and reopen:

```sh
fz card move 87 --to Now
fz card move 87 --to maybe
fz card move 87 --to not-now
fz card move 87 --to done
fz card move 87 --board Garden --to Next
fz card close 87
fz card reopen 87
```

`--to` takes a column name, or `maybe`, `not-now` or `done`. An exact column
name wins. A name that two columns share in any case, with no exact match,
exits 2. `--board` moves the card to another board.

`card close` closes a card. It sends the same call as `--to done`, but it
never reads the board's columns. A card that is already closed needs no
change, so `fz` says so, sends nothing and exits 0. `card close` takes
`--dry-run` and `--json` and needs no `--yes`. `card reopen` moves a closed
card out of Done.

A board can have a column of its own named Done, Maybe, Maybe? or Not now.
Then `--to` with that name could mean the column or the keyword, so `fz`
refuses it with exit 2 and sends nothing. Case does not matter. To move the
card into that column, give `--to` the column's id. The refusal names it,
and `fz --json column list Home` prints it. To close the card, use
`fz card close 87`. To send it to Maybe? or Not now with the keyword, rename
the column first. `card create --column`, `draft apply` and `draft apply --update`
refuse the same way, before they send anything.

Golden:

```sh
fz card golden 87
fz card golden 87 --off
```

Golden cards are a board's current priorities.

Tag:

```sh
fz card tag 87 waiting
fz card tag 87 waiting --remove
```

Fizzy's tag call toggles a tag on or off. `fz` reads the card first and only
sends a change when one is needed, so tagging twice never removes the tag.
If the card in Fizzy's reply has no tags list, `fz` cannot tell, so it exits
2 and sends nothing. An empty tag exits 2 as well.

#### Assign

```sh
fz card assign 87 "Gate Keeper"
fz card assign 87 03abc123def456ghi789jkl0mn
fz card unassign 87 "Gate Keeper"
```

USER is a user id, a name or an email address. `fz` tries, in order, an exact
id, an exact name, a name with any case when only one user has it, and an
email with any case. If two users have the name, or no user matches, `fz`
refuses and exits with status 2. It prints which user it matched and how.
`card unassign` also takes an id that matches no active user and sends it as
it is, so a deactivated user can be removed. `card assign` does not.

Fizzy's assign call toggles, like the tag call. `fz` reads the card first and
only sends a change when one is needed. The card lists at most 5 assignees.
When a card has more and the user is not among the 5, `fz` refuses, because
it cannot tell whether that user is assigned.

#### Watch and pin

```sh
fz card watch 87
fz card unwatch 87
fz card pin 87
fz card unpin 87
fz card pins
```

Watching a card subscribes you to its notifications. A pin is yours alone.
`card pins` lists your pinned cards. Fizzy returns at most 100 of them and
does not page them.

#### Boosts

```sh
fz card boost 87 "Nice hinge"
fz card boosts 87
fz card unboost 87 03abc123def456ghi789jkl0mn
```

A boost is a short reaction on a card, at most 16 characters. `fz` counts
characters as Fizzy does, one per Unicode code point, and refuses longer text
before sending it. `card boosts` prints each boost's id, text and who added
it. You can only remove your own boosts.

#### Header image

```sh
fz card image 87 --remove --yes
```

`fz` can only remove a card's header image, so `--remove` is required. It
prints the card first and refuses without `--yes`. A card with no image
prints that and exits 0. If Fizzy's reply has no `image_url` field, `fz`
cannot tell whether there is an image, so it exits 2 and sends nothing.

Delete:

```sh
fz card delete 87 --yes
```

`fz` prints the card first. Without `--yes` it deletes nothing and exits with
status 2. A deleted card is gone for good.

### Steps

Steps are the checklist on a card. Each one has an id, which `fz step list`
prints.

```sh
fz step list 87
fz step add 87 "Order the hinge"
fz step add 87 "Measure the gap" --done
fz step check 87 03abc123def456ghi789jkl0mn
fz step uncheck 87 03abc123def456ghi789jkl0mn
fz step edit 87 03abc123def456ghi789jkl0mn "Order two hinges"
fz step delete 87 03abc123def456ghi789jkl0mn --yes
```

`step add` puts the step at the end of the list. `--done` adds it already
ticked. `step delete` needs `--yes`, like a card delete. Without it, `fz`
prints the step it would delete, deletes nothing and exits with status 2.

### Comments

```sh
fz comment add 87 "Measured the gap: 42 mm"
fz comment list 87
fz comment show 87 03abc123def456ghi789jkl0mn
fz comment edit 87 03abc123def456ghi789jkl0mn "Gate latches now."
fz comment delete 87 03abc123def456ghi789jkl0mn --yes
```

The text is Markdown. `comment list` prints each comment's id, author, time
and first line, oldest first. `comment show` prints one comment in full.
`comment edit` replaces the whole text. `comment delete` prints the comment
first and refuses without `--yes`. The Fizzy API docs say only a comment's
creator can edit or delete it. When the server answers 403, `fz` prints the
status and adds "only the comment's author can edit or delete it", and exits
1.

Reactions:

```sh
fz comment react 87 03abc123def456ghi789jkl0mn "ok"
fz comment reactions 87 03abc123def456ghi789jkl0mn
fz comment unreact 87 03abc123def456ghi789jkl0mn 03def456ghi789jkl0mn123abc
```

A reaction is at most 16 characters, counted as boosts are. Empty, blank or
longer text exits 2 and sends nothing. Fizzy does not send back the new
reaction's id, so `comment react` prints the comment id, and its `--json`
output holds the comment id and the text. `comment reactions` prints each
reaction's id, who reacted and the text. `comment unreact` removes your own
reaction.

### Upload

```sh
fz upload hinge.jpg
```

This uploads a file through Fizzy's direct upload and prints its
`attachable_sgid`, which embeds the file in rich text. Fizzy's API docs say
to embed the `signed_id` instead, but that renders as a broken attachment.
Drafts upload their images for you, so most people never need this command.

### Tags and people

```sh
fz tag list
fz user list
fz user show 03abc123def456ghi789jkl0mn
fz user show Alex
fz whoami
```

`tag list` prints every tag on the account with its id. `user list` prints
the active users with their id, name, email and role. `user show` and
`user edit` take a user id, a name or an email, matched the way
`card assign` matches a user. A name two users share, or a USER that
matches nobody, exits 2. An id that
matches no active user is still tried, so a deactivated user's id works.
`whoami` lists every
account your token can reach, with your user on each, and marks the
configured account with `*`.

Change a name or your timezone:

```sh
fz user edit 03abc123def456ghi789jkl0mn --name "Gate Keeper" --yes
fz user timezone America/Denver
```

`user edit` prints the current and new name and refuses without `--yes`.
When the user already has that name, it prints that no change is needed,
exits 0 and sends nothing, with or without `--yes`.
`user timezone` takes an IANA name, which Fizzy uses for times in
notification emails. Before sending it, `fz` checks the name against the
list of zones built into Go, case included, so `america/denver` exits 2 on
every system.

### Notifications and activity

```sh
fz notification list
fz notification read 03abc123def456ghi789jkl0mn
fz notification read --all
fz notification unread 03abc123def456ghi789jkl0mn
fz notification settings
fz notification settings --email daily
```

`notification list` prints unread notifications first, then read ones.
`notification read --all` marks every unread one read. `--email` sets how
often Fizzy bundles notification emails: `never`, `every_few_hours`, `daily`
or `weekly`.

```sh
fz activity list
fz activity list --limit 0
fz activity list --board Home --creator 03abc123def456ghi789jkl0mn
```

`activity list` prints activity newest first, with the time, the action, what
it happened to and a short summary. It stops after `--limit` entries, 50 by
default. `--limit 0` reads every page. `--creator` matches a user the way
[card assign](#assign) does. An id that matches no active user is sent as it
is, so a deactivated user's activity can still be listed.

### Account and exports

```sh
fz account show
fz account postpone 30 --yes
```

`account show` prints the account's name, id, card count, default
auto-postpone period and creation time. `account postpone` sets that default
for boards that do not set their own. It takes 3, 7, 11, 30, 90 or 365 days,
prints the current and new period, and refuses without `--yes`. When the
period is already DAYS, it prints that no change is needed, exits 0 and
sends nothing, with or without `--yes`. It needs the admin role.

```sh
fz export account
fz export mine
fz export status 03abc123def456ghi789jkl0mn
fz export status 03abc123def456ghi789jkl0mn --mine
```

`export account` starts an export of the whole account. Only admins and
owners can. `export mine` starts an export of your own data. `export status`
prints the export's id, status and creation time, and the download link once
it is done. Add `--mine` for a personal export.

### Webhooks

A webhook sends a board's events to a URL. The Fizzy API docs say only
account admins can use them. A board is named by its name or its id.

```sh
fz webhook list Gate
fz webhook show Gate 03abc123def456ghi789jkl0mn
fz webhook deliveries Gate 03abc123def456ghi789jkl0mn
```

`webhook list` and `webhook show` never print the signing secret, even with
`--json`. `webhook show --json --reveal-secret` includes it. `webhook
deliveries` lists each attempt to send an event.

Create, change and remove:

```sh
fz webhook create Gate --name "Hinge alerts" --url https://fizzy.example.com/hook --actions card_closed,comment_created --yes
fz webhook edit Gate 03abc123def456ghi789jkl0mn --actions card_closed --yes
fz webhook edit Gate 03abc123def456ghi789jkl0mn --name "Gate alerts" --yes
fz webhook delete Gate 03abc123def456ghi789jkl0mn --yes
fz webhook activate Gate 03abc123def456ghi789jkl0mn
```

- The URL must be `https`, or `http` for `localhost`, `127.0.0.1` or `::1`.
  It cannot change after the webhook is created.
- `--actions` is a comma-separated list. The actions are `card_assigned`,
  `card_closed`, `card_postponed`, `card_auto_postponed`,
  `card_board_changed`, `card_published`, `card_reopened`,
  `card_sent_back_to_triage`, `card_triaged`, `card_unassigned` and
  `comment_created`.
- `create`, `edit` and `delete` print what they would do and refuse without
  `--yes`.
- `webhook create` prints the signing secret once. Save it then. In text
  it goes to stderr, after the warning to save it, so stdout holds only the
  webhook id. With `--json` it is in the JSON on stdout, and the warning
  goes to stderr.
- `webhook activate` turns a deactivated webhook back on. It sends nothing
  when the webhook is already active.

### Review

```sh
fz review
fz review Home Garden
fz --json review Home
```

For each board, `review` lists the cards in the last column before Done and
counts the cards in the other columns and in Maybe?. It also lists golden
cards, cards about to be postponed, and stalled cards. Closed cards are left
out. With no board names it reviews every board.

### Man pages

```sh
fz man
fz man --dir /tmp/fz-man
```

This writes one man page per command, such as `fz.1` and `fz-card-list.1`,
into `DIR/man1`. The default DIR is `~/.local/share/man`. It needs no config
file.

### Shell completion

```sh
fz completion zsh
fz completion bash
```

This prints a completion script. `fish` and `powershell` also work. Run
`fz completion zsh --help` for how to load the script.

### Help

```sh
fz help
fz help card
fz card edit --help
```

### Commands from fz 0.1

The older command forms still work:

```sh
fz boards
fz show 87
fz move 87 --to Now
fz tag 87 buy
fz tag --remove 87 buy
fz step 87 "Order the hinge"
fz step 87 --check 03abc123def456ghi789jkl0mn
fz comment 87 "Measured the gap: 42 mm"
```

The full list is `boards`, `board-create`, `columns`, `cards`, `show`,
`create`, `move`, `reopen`, `golden`, `delete` and `tag`, plus the
numbered forms of `step` and `comment`. The global flags can come first, in
any of their forms, such as `fz --json=true cards`.

## Drafts

A draft is one Markdown file per card. Write it in any editor, check it,
preview it, then apply it to create the card.

### The format

```markdown
---
board: Home
column: Next
tags: buy, gate
golden: false
---
# Fix the side gate

## Goal
The gate closes and latches by itself.

## Details
The hinge pin is worn. The latch is fine.

## Links & media
- [Hinge spec](https://example.com/hinge)

![The worn hinge](hinge.jpg)

## Steps
- [ ] Order the hinge
- [ ] Swap it
- [x] Measure the gap
```

- The front matter needs `board`. `column`, `tags` and `golden` are
  optional. A draft with no column goes to Maybe?. When the board has a column of
  its own named like a keyword, a draft that would move the card there by
  that name is refused, as [`card move`](#cards) refuses it.
- The `# ` line is the card title.
- Everything except the title and the Steps section becomes the description.
- Each `- [ ]` or `- [x]` line under `## Steps` becomes a step, ticked or not.
- `skill/templates/card.md` in this repo is a blank draft to copy.

### Images

An image line, `![alt](file.jpg)`, must sit on a line by itself. Inside a
list item it is refused, because Fizzy would show it as text.

Every image line names a local file. A relative path is looked up next to the
draft, then under the inbox, so `gate/hinge.jpg` can mean
`~/.fizzy-inbox/gate/hinge.jpg`. A web image cannot be embedded. Link to it
with a normal link instead.

`apply` uploads each image and embeds it in the description.

### Check, preview, apply

```sh
fz draft check gate.md
fz draft check gate.md --links
fz draft preview gate.md
fz draft apply gate.md
fz draft apply ~/.fizzy-inbox/drafts/
```

- `check` parses each draft and finds its images. `--links` also fetches
  every link and reports its status. Those fetches carry no token.
- `preview` prints the title, board, column, tags, description HTML and steps.
  It sends nothing.
- `apply` parses every draft and finds its images before it creates
  anything. Then it creates one card per draft and writes `card: N` into the
  draft, so the same draft never makes a second card.
- A draft that already has `card:` is skipped, unless you pass `--update`.
- Each command takes files, folders or both. A folder means the `.md` files
  directly inside it.

### Updating a card from its draft

```sh
fz --dry-run draft apply --update gate.md
fz draft apply --update gate.md
fz draft apply --update --prune gate.md
fz draft apply --update --force gate.md
```

`--update` pushes the draft's board, title, description, tags, column, golden
mark and steps to the card named by `card:`. Run it with `--dry-run` first to
see what it would send.

How steps are matched:

- A draft step matches a card step with the same text.
- A matched step is left alone. If the draft ticks it, the card step is
  ticked.
- Nothing is ever unticked. A tick made in Fizzy is newer than the draft.
- A draft step with no match is added. Changing a step's text in the draft
  therefore adds a new step and leaves the old one.
- Card steps and tags the draft does not have are kept and reported.

`--prune` deletes those extra card steps if they are unticked, and removes the
extra tags. Ticked steps always stay. A closed card is never reopened, and
images are uploaded again on every update.

### The synced fingerprint

After every apply, `fz` writes two more lines into the draft's front matter.

- `synced:` is a fingerprint of the card as Fizzy then holds it. It covers
  the title, description, steps, tags, golden mark, column, board, closed
  state and Not now state.
- `synced-fields:` holds a short hash of each of those fields.

Leave both lines alone. On the next `--update`, `fz` reads the card and
compares. If someone changed the card in Fizzy since the last sync, `fz`
refuses the draft and names what changed:

```text
fz: gate.md: card #87 changed in Fizzy since the last sync (description, steps); check it, then rerun with --force
```

A refusal means the update would overwrite an edit made in Fizzy. Look at the
card with `fz card show 87`. Copy anything you want to keep into the draft,
then rerun with `--force`. A draft written by an older `fz` has no
`synced-fields:` line, so its refusal does not name the fields.

`fz` also refuses a draft that has no `synced:` line, or one that holds
something other than a fingerprint, because it cannot tell what changed.
`--force` overrides both.

`apply --update` checks every draft before it writes anything, so one refused
draft stops the whole run and nothing is half applied.

## The skill

The fizzy-cards skill lets Claude Code build cards with you. It needs only
`fz` on your PATH.

### Install the skill

Link the repo's `skill/` folder into your Claude Code skills folder:

```sh
mkdir -p ~/.claude/skills
ln -s "$PWD/skill" ~/.claude/skills/fizzy-cards
```

Run that from the root of a checkout. The folder holds `SKILL.md` and
`templates/card.md`.

### What it does

Give it a card title and a board. The skill then:

1. Checks that `fz` is installed and can reach your server, and reads your
   rules card if you set one.
2. Asks three to five questions, one at a time. It always asks how deep to
   research: quick, a few key links, or deep, comparing options.
3. Researches to that depth, preferring primary sources.
4. Writes a draft to `<inbox>/drafts/<card-slug>.md` from the template.
5. Runs `fz draft check --links` and fixes or removes every link that fails.
6. Shows you the draft in chat.
7. Runs `fz draft apply` when you say go, and gives you the card's link.

Later changes go through the same draft and `fz draft apply --update`. The
skill asks before it uses `--prune`.

For a list of titles, it works in batch mode. It asks once which board the
list belongs to, drafts every card into one folder, checks them, and shows a
compact list. One go creates them all.

The skill never creates, changes, moves or deletes anything in Fizzy until
you approve that draft or batch. It never prints your token.

### While you work in a mapped repo

When a session's work matches a card:

- After you finish one of its steps, the skill offers once to tick it.
- When the card's goal is met, it asks before moving the card to Done.
- It never creates, moves, ticks or deletes anything without your OK.

### Your rules card

Set `rules_card` in the config to a card holding your own conventions. The
skill reads that card first and follows it over its own defaults. It ignores
any part of the card that would loosen its golden rules.

## The session hook

`fz hook session-start` shows the open cards for the repo a Claude Code
session starts in. It prints the cards in Now and Next on that repo's board,
and Claude Code adds them to the session. If the map entry names a card, it
prints only that card.

### Turn it on

Add this to `~/.claude/settings.json`:

```json
{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "fz hook session-start"}]}]}}
```

The last line it prints tells Claude to follow the skill's rules. `--note`
changes that line, and `--note ""` drops it:

```sh
fz hook session-start --note "Ask before moving a card."
```

### The repos map

The hook reads `~/.config/fizzy-skill/repos.json`, or the file named by
`FIZZY_REPOS_MAP`. Each key is a folder name:

```json
{
  "gatehouse": {"board": "Home"},
  "latch": {"board": "Home", "card": 87}
}
```

Inside a git worktree, the main checkout's folder name is tried first. Then
each folder in the path is tried, from the deepest up. The first name found
in the map wins.

A folder that is not in the map prints nothing and reads no config. Any
failure also prints nothing and exits 0, so the hook never blocks a session.
That covers a missing map, a config error, a network error and the 5 second
time limit.

### The Python version

`examples/hooks/fizzy-session-hook.py` does the same job in Python 3. Use it
or `fz hook session-start`, not both. `examples/hooks/README.md` covers its
setup.

## Troubleshooting

### Config errors

| Message | Fix |
|---|---|
| `set FIZZY_URL and FIZZY_ACCOUNT, or url/account in <file>` | The file is missing or lacks `url` or `account`. Check the path `fz` names. |
| `<file>: invalid JSON (...)` | Fix the JSON in the config file. |
| `rules_card "<value>" is not a number` | `rules_card` must be a card number. |
| `<file>: invalid JSON (text after the JSON object)` | Remove whatever follows the closing `}`. |
| `<file>: the key "<key>" appears twice` | Keep one copy of the key. |
| `<file>: <key> must be a string or a number` | Give the key a value in quotes, or a number for `rules_card`. `true`, a list or an object is refused. |
| `<file>: the config file cannot hold the token; ...` | Remove the `token` key. Put a command that prints the token in `token_cmd`, or set `FIZZY_TOKEN`. |
| `<file> is writable by group or others, ...` | Run the `chmod go-w` command the message gives. |
| `the server URL must start with https://` | Put `https://` in front of the host. |
| `the server URL must use https (plain http is allowed only for localhost)` | Use the https address. |
| `the server URL must not hold a user name or password` | Remove the `name:password@` part. The token comes from the token command. |
| `the server URL must not have a query` or `a fragment`, or `has no host` | Set `url` to `https://HOST`, with an optional path and nothing after it. |
| `the account is empty`, `must not contain spaces`, `must not contain /, ?, # or %` or `must not contain ..` | Set `account` to the account part of your Fizzy URLs, such as `1`. |
| `no token: set FIZZY_TOKEN or FIZZY_TOKEN_CMD (or token_cmd in the config file)` | Give `fz` a token or a token command. |

### Token command failures

`token command failed` means the command did not work. The message gives
the reason in parentheses:

| Reason | What happened |
|---|---|
| `exit status N` | The command ran and exited with status N. |
| `not found` | There is no such command. Check the first word of `token_cmd`. |
| `timed out after 30s` | The command ran for 30 seconds without finishing. A keychain prompt that nobody answered does this. |
| `invalid command` | `token_cmd` has an unclosed quote, or holds only spaces. |

`token command printed nothing` means it ran but printed no token.

The message never includes anything the command printed, on stdout or on
stderr, because a broken command may print the token on either one. Run the
command yourself in a terminal to see its error.

### Timeouts

A request that takes longer than 60 seconds fails. Sending a file's bytes,
for `fz upload` or a draft image, gets 10 minutes instead, so a large file
on a slow link can finish. If every command times
out but the server works in a browser, something on your machine is
blocking `fz`. On macOS, check Local Network permission in System Settings,
under Privacy & Security, Local Network. Check any outbound firewall, such
as Little Snitch, too. A rebuilt binary may need permission again.

### "Changed in Fizzy" refusals

`fz draft apply --update` refuses a draft when the card changed in Fizzy
since the last sync. This protects the change made in Fizzy. Read the card,
move what you want to keep into the draft, then rerun with `--force`. See
[The synced fingerprint](#the-synced-fingerprint).

### The action log

`fz` appends one JSON line per write to
`~/.local/state/fizzy-skill/actions.log`:

```json
{"t": "2026-10-17T15:04:05Z", "method": "POST", "path": "/1/cards/87/steps", "status": 201}
```

`t` is the time in UTC. `method` and `path` name the request. `status` is the
HTTP status the server returned. A status of 300 or above means the write
failed. A status of 0 means the write got no reply, for example after a
timeout or a dropped connection, so the server may or may not have applied
it. Check in Fizzy before you run it again. The log never holds the token or
the request and response bodies. A dry run sends nothing, so it logs nothing.

`fz` creates the log with mode 0600, so only you can read it, and sets an
existing log to 0600.

### A redirected write

`POST /1/boards -> HTTP 301: the server redirected a write, and fz does not
follow that; the configured URL may need https` means the server answered a
write with a redirect. The status can also be 302 or 303. Following it would turn the write into a read, so `fz` stops and
nothing changes. This usually means `url` names an http address that
redirects to https. Set `url` to the https address. `fz` never follows a
redirect from https to http either.
