---
name: agent-maid
description: >-
  Use this skill to inspect and switch the local AI tool accounts managed by
  agent-maid: ChatGPT/Codex (auth.json account library), ZCode (credential
  slots), and Antigravity/agy CLI (OAuth token slots). Trigger when the user
  asks which account a tool is logged in as, wants to switch/rotate accounts,
  save the currently logged-in account, list saved accounts, import a Codex
  session JSON, delete old accounts, or log out so they can sign in as someone
  else. Also trigger on "agent-maid", "账号切换", "切换到 xxx 账号",
  "我现在用的哪个账号", "把当前账号存下来", and when a Codex/ZCode/agy task
  fails with account/quota/auth errors and switching accounts is a plausible fix.
---

# agent-maid

Local, offline account manager for three AI tools. It never talks to the
network: it snapshots and restores **local credential files**, so switching
accounts is a file swap plus (for the app) a restart.

| Target key | App | Credential file agent-maid writes |
|---|---|---|
| `chatgpt` | ChatGPT / Codex CLI | `~/.codex/auth.json` (or `$CODEX_HOME`) |
| `zcode` | ZCode desktop | `~/.zcode/v2/credentials.json` (or `$ZCODE_V2_DIR`) |
| `antigravity` | agy (Antigravity CLI) | `~/.gemini/antigravity-cli/antigravity-oauth-token` |

## Run the commands

Always prefer the headless subcommands with `--json`. They need no TTY and are
safe to call from an agent shell.

```bash
agent-maid list    <target> --json    # accounts/slots + which one is live
agent-maid current <target> --json    # what the app is logged in as right now
agent-maid use     <target> <name>    # switch/activate
agent-maid save    <target> [name]    # snapshot the live credential
agent-maid rm      <target> <name>    # delete (goes to trash)
agent-maid import  chatgpt <file|->   # import Codex session JSON (chatgpt only)
agent-maid logout  <target>           # clear local credential, keep account saved
```

If `agent-maid` is not on `PATH`, use the repository build
(`./agent-maid` / `./dist/agent-maid`) before concluding it is missing.

Exit codes: `0` success, `1` operation failed, `2` usage mistake (wrong
target/arguments). On failure the reason is in `.error` for `--json` and on
stderr otherwise. **Read the error instead of retrying blind.**

Only the interactive TUI (`agent-maid`, `agent-maid <target>`) manages accounts
by keyboard; never launch it from a non-interactive shell — it exits with code 2.

## The one workflow that matters

To switch accounts, the account must already be saved. The reliable sequence:

1. `agent-maid current <target> --json` — see who is logged in and whether it
   is already saved (`current.in_library` for chatgpt, `in_use` flags in the
   list for slots).
2. If it is **not** saved, `agent-maid save <target> <meaningful-name>` first.
   `use` **refuses** to switch away from an unsaved account (exit 1,
   `reason: "unsaved_current"`), because the outgoing credential would
   otherwise survive only as an unidentifiable `auto-<timestamp>` slot.
   Do not reach for `--force` to skip this: it still creates that slot and
   loses track of whose account it was. Only pass `--force` when the user has
   explicitly said the current login is disposable.
3. `agent-maid use <target> <name>`.
4. Tell the user to **restart the app** (see caveats). For `zcode` and
   `antigravity` the app may have already overwritten the file on exit.

Never `rm` an account without the user naming it explicitly — deletion cannot
be undone once the process exits.

## Reading the JSON

All outputs share `{"type","ok","error?"}`. Key fields:

- `list` (chatgpt): `total`, `accounts[]` with `label`, `email`,
  `account_id`, `plan`, `refreshable`, `in_use`; plus `current.in_library`.
- `list` (zcode/antigravity): `total`, `slots[]` with `name`, `in_use`,
  `last_used_at`, and for agy `email` / `refreshable` / `expired`.
  `running` reports whether the app is up right now.
- `current`: `exists`, `email`, plus target-specific fields
  (`account_id`, `auth_mode`, `auth_path`, `auth_method`, `expires_at`,
  `refreshable`, `expired`).
- `use`: `slot`/`account` name and `auto_backup` when a backup was made
  (only possible with `--force`).
- `use` on an unsaved current account fails with
  `{"ok":false,"reason":"unsaved_current","needs":"save",
    "suggested_save":"agent-maid save chatgpt <name>","force_flag":"--force"}`
  — follow `needs` instead of retrying.
- `import`: `imported`, `skipped`, `bad_docs`.

`in_use` means "this is what the app currently reads", which after a manual
`use` is authoritative even if the app is still running with old credentials in
memory.

## Caveats to relay to the user

- **Restart the app after switching.** Codex reads `auth.json` at startup;
  ZCode and agy hold credentials in memory and rewrite the file on exit, which
  can clobber the switch. `list --json` has `running` — if true, say so.
- **`refreshable: false` means the credential will expire and cannot renew.**
  For chatgpt that is a missing `refresh_token`; for agy the same. Say the
  user will need to log in again rather than pretending the account is fine.
- **`logout` does not revoke server-side tokens.** It clears the local file so
  the app shows a login screen. This is deliberate: logging out inside the app
  can revoke the token and kill the saved slot.
- **agy logout renames instead of deleting** (`.antigravity-oauth-token.logged-out-*`),
  matching agy's own behaviour, so a mistaken logout is recoverable.
- `import` is chatgpt-only. ZCode and agy accounts can only be captured from a
  live login via `save`; there is no file format to import.

## Reference

- Exact command help and JSON schemas: [references/cli.md](references/cli.md)
- Where state lives, env vars, and reset/repair recipes:
  [references/state.md](references/state.md)
- If a command fails unexpectedly, run `agent-maid help` and compare with
  [references/cli.md](references/cli.md) — a mismatch means the installed
  binary is older than this skill.
