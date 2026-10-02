"""Configuration: environment variables override a JSON config file.

Config file (default ~/.config/fizzy-skill/config.json, or $FIZZY_CONFIG):

    {
      "url": "https://fizzy.example.com",
      "account": "1",
      "token_cmd": "security find-generic-password -s fizzy-api -w",
      "inbox": "~/.fizzy-inbox",
      "rules_card": 87
    }

Environment variables: FIZZY_URL, FIZZY_ACCOUNT, FIZZY_TOKEN, FIZZY_TOKEN_CMD,
FIZZY_INBOX, FIZZY_RULES_CARD.

The token is never stored by this tool: give it directly in FIZZY_TOKEN, or a
command that prints it (FIZZY_TOKEN_CMD / "token_cmd"), e.g. a keychain lookup.
"""

from __future__ import annotations

import json
import os
import shlex
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


class ConfigError(Exception):
    pass


DEFAULT_CONFIG_PATH = Path("~/.config/fizzy-skill/config.json")
DEFAULT_INBOX = "~/.fizzy-inbox"
DEFAULT_LOG = Path("~/.local/state/fizzy-skill/actions.log")


@dataclass
class Config:
    url: str
    account: str
    token_cmd: Optional[str] = None
    token: Optional[str] = None
    inbox: str = DEFAULT_INBOX
    rules_card: Optional[int] = None
    log_path: Path = DEFAULT_LOG

    @property
    def inbox_path(self) -> Path:
        return Path(self.inbox).expanduser()

    def resolve_token(self) -> str:
        if self.token:
            return self.token
        if not self.token_cmd:
            raise ConfigError("no token: set FIZZY_TOKEN or FIZZY_TOKEN_CMD (or token_cmd in the config file)")
        try:
            out = subprocess.run(shlex.split(self.token_cmd), capture_output=True, text=True, check=True)
        except (OSError, subprocess.CalledProcessError) as exc:
            # Never echo command output: it may contain the secret.
            raise ConfigError(f"token command failed ({type(exc).__name__})") from None
        token = out.stdout.strip()
        if not token:
            raise ConfigError("token command printed nothing")
        return token


def load(path: Optional[str] = None) -> Config:
    cfg_path = Path(path or os.environ.get("FIZZY_CONFIG") or DEFAULT_CONFIG_PATH).expanduser()
    data = {}
    if cfg_path.exists():
        try:
            data = json.loads(cfg_path.read_text())
        except json.JSONDecodeError as exc:
            raise ConfigError(f"{cfg_path}: invalid JSON ({exc})") from None

    env = os.environ
    url = env.get("FIZZY_URL") or data.get("url")
    account = env.get("FIZZY_ACCOUNT") or data.get("account")
    if not url or not account:
        raise ConfigError(f"set FIZZY_URL and FIZZY_ACCOUNT, or url/account in {cfg_path}")
    rules = env.get("FIZZY_RULES_CARD") or data.get("rules_card")
    return Config(
        url=str(url).rstrip("/"),
        account=str(account).strip("/"),
        token_cmd=env.get("FIZZY_TOKEN_CMD") or data.get("token_cmd"),
        token=env.get("FIZZY_TOKEN") or None,
        inbox=env.get("FIZZY_INBOX") or data.get("inbox") or DEFAULT_INBOX,
        rules_card=int(rules) if rules else None,
    )
