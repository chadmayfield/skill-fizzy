#!/usr/bin/env python3
"""SessionStart hook: show the open Fizzy cards for the repo this session is in.

Claude Code passes the session JSON on stdin; whatever this prints on stdout
is added to Claude's context. The repo -> board map lives in
~/.config/fizzy-skill/repos.json. A folder that is not in the map prints
nothing. Any failure (no map, server down, fz slow) also prints nothing:
this hook must never get in the way of starting a session.
"""
import json
import os
import subprocess
import sys

MAP = os.path.expanduser(os.environ.get("FIZZY_REPOS_MAP", "~/.config/fizzy-skill/repos.json"))
FZ = os.path.expanduser(os.environ.get("FIZZY_FZ", "~/.local/bin/fz"))
TIMEOUT = 5  # seconds per fz call; fz normally answers in ~0.5 s


def run(*args):
    out = subprocess.run(args, capture_output=True, text=True, timeout=TIMEOUT)
    out.check_returncode()
    return out.stdout.strip()


def repo_names(cwd):
    """Candidate names for this folder, most specific first.

    Inside a git worktree (.claude/worktrees/x) the repo is the main
    checkout, found through --git-common-dir.
    """
    names = []
    try:
        common = run("git", "-C", cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
        if common.endswith("/.git"):
            names.append(os.path.basename(os.path.dirname(common)))
    except Exception:
        pass
    names += [p for p in reversed(cwd.split(os.sep)) if p]
    return names


def main():
    try:
        cwd = json.load(sys.stdin).get("cwd") or os.getcwd()
    except Exception:
        cwd = os.getcwd()
    with open(MAP) as f:
        repos = json.load(f)
    entry = next((repos[n] for n in repo_names(cwd) if n in repos), None)
    if not entry:
        return

    board = entry["board"]
    cards = json.loads(run(FZ, "--json", "cards", "--board", board))
    focus = entry.get("card")

    def line(c):
        col = (c.get("column") or {}).get("name") or "Maybe?"
        star = " ★" if c.get("golden") else ""
        return f"  #{c['number']} [{col}] {c['title']}{star}"

    # A repo mapped to one card shows only that card; the rest of the
    # board has nothing to do with the repo.
    if focus:
        picked = [c for c in cards if c["number"] == focus]
        print(f"Fizzy: this repo's card is on the {board} board:")
        print("\n".join(line(c) for c in picked) or f"  #{focus} (not open)")
        print("Follow the fizzy-cards skill's rules for moving cards and ticking steps.")
        return

    picked = [c for c in cards if (c.get("column") or {}).get("name") in ("Now", "Next")]
    order = {"Now": 0, "Next": 1}
    picked.sort(key=lambda c: order.get((c.get("column") or {}).get("name"), 2))

    print(f"Fizzy: this repo maps to the {board} board.")
    if picked:
        print("Open cards in Now and Next:")
        print("\n".join(line(c) for c in picked))
    else:
        print("Nothing in Now or Next.")
    print("Follow the fizzy-cards skill's rules for moving cards and ticking steps.")


if __name__ == "__main__":
    try:
        main()
    except Exception:
        pass
    sys.exit(0)
