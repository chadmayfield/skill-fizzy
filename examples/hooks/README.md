# Session hook example

`fizzy-session-hook.py` is a Claude Code SessionStart hook. When a session
starts in a folder listed in your repos map, it prints that board's open
cards in Now and Next, and Claude Code adds them to the session. If the map
entry names a card, it prints only that card. An unmapped folder prints
nothing, and so does any failure, so the hook never blocks a session.

`fz hook session-start` does the same job without Python. Use one or the
other.

## What it needs

- Python 3.
- `fz` on your PATH, set up so that `fz board list` works. The script runs
  `~/.local/bin/fz` unless `FIZZY_FZ` names another path.
- A repos map at `~/.config/fizzy-skill/repos.json`, or at the path in
  `FIZZY_REPOS_MAP`. Each key is a repo or folder name:

  ```json
  {
    "gatehouse": {"board": "Home"},
    "latch": {"board": "Home", "card": 87}
  }
  ```

  Inside a git worktree, the main checkout's folder name is tried first. Then
  each folder in the path is tried, from the deepest up.

## Setup

1. Copy the script to `~/.claude/hooks/fizzy-session-hook.py` and make it
   executable: `chmod +x ~/.claude/hooks/fizzy-session-hook.py`.
2. Add it to `~/.claude/settings.json`:

   ```json
   {
     "hooks": {
       "SessionStart": [
         {"hooks": [{"type": "command", "command": "~/.claude/hooks/fizzy-session-hook.py", "timeout": 15}]}
       ]
     }
   }
   ```

To use the Go command instead, set `"command"` to `"fz hook session-start"`.
Its closing line points at the fizzy-cards skill; `--note "<text>"` changes
it and `--note ""` drops it. The script prints the same line; edit it
there to change it.
