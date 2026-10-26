# agent-maid CLI reference

Everything here is verified against the binary in this repository. Each command
accepts `--json` in any position (before the command, after the target, or at
the end) and never requires a TTY.

## Commands

### `list <target>`

Show every saved account/slot and mark the live one.

```
agent-maid list chatgpt
agent-maid list zcode --json
```

```json
{"type":"list","ok":true,"target":"chatgpt","total":1,
 "accounts":[{"id":"acc-1","label":"work","email":"a@example.com",
              "account_id":"acc-1","plan":"plus","source":"auth.json",
              "refreshable":true,"in_use":true,
              "last_used_at":"2026-10-26T04:04:00+08:00"}],
 "current":{"exists":true,"auth_mode":"chatgpt","email":"a@example.com",
            "account_id":"acc-1","path":"/root/.codex/auth.json",
            "last_refresh":"2026-10-26T00:00:00Z","in_library":true}}
```

`accounts[]` fields: `id`, `label`, `email`, `account_id`, `plan`, `source`,
`refreshable`, `in_use`, `last_used_at`, `expires_at` (last two only when
known). `current` is omitted when nothing is logged in.

Empty store is success (`ok:true`, `total:0`) with a hint on stdout — not an
error. Text mode prints an aligned table.

### `current <target>`

What the app is logged in as right now, read from the live credential file.

```json
{"type":"current","ok":true,"target":"antigravity",
 "exists":true,"email":"user@example.com","auth_method":"consumer",
 "refreshable":true,"expires_at":"2026-10-26T06:43:49+08:00",
 "expired":false,
 "path":"/root/.gemini/antigravity-cli/antigravity-oauth-token",
 "running":false}
```

The result is wrapped: `{"type":"current","ok":true,"target":…,"current":{…}}`,
and `current` is `null` when nothing is logged in.

`exists:false` (with `path`) means not logged in — report that instead of
treating it as a failure.

### `use <target> <name> [--force]`

Switch to a saved account. `<name>` accepts the case-insensitive exact label
or account_id, or a unique prefix (chatgpt). For slots it must be the slot name.

**If the currently logged-in account is not saved, `use` refuses** (exit 1)
rather than silently leaving it behind as an `auto-<timestamp>` slot:

```json
{"type":"error","ok":false,"reason":"unsaved_current",
 "error":"当前登录的账号 b@example.com 尚未保存到账号库",
 "target":"chatgpt","needs":"save",
 "suggested_save":"agent-maid save chatgpt <name>","force_flag":"--force"}
```

Run the `save` it suggests, then retry. `--force` overrides the refusal and
performs the old automatic backup — use it only when the current login is
known to be disposable, because the backup slot's name will not identify
whose account it holds.

```json
{"type":"use","ok":true,"target":"zcode","slot":"work",
 "auto_backup":"auto-1026-054927","zcode_running":true,
 "warning":"ZCode 正在运行:切换后建议尽快重启,避免运行中的凭据回写覆盖"}
```

For `chatgpt`, `use` returns the account object plus `path` instead of `slot`:

```json
{"type":"use","ok":true,"target":"chatgpt","path":"/root/.codex/auth.json",
 "account":{"id":"acc-1","label":"work","account_id":"acc-1","plan":"plus",
            "refreshable":true,"in_use":true}}
```


`auto_backup` only appears with `--force`; it names the `auto-*` slot created
for the outgoing unsaved account. `warning` plus `*_running` mean: tell the
user to restart the app.

### `save <target> [name]`

Snapshot the live credential into the account library / a new slot.

Omit `name` for chatgpt to keep the previously saved label (so re-saving after
a token refresh does not rename the user's account back to their email). For
slots the name is required in practice — omitted it falls back to the account's
provider/username.

```json
{"type":"save","ok":true,"target":"chatgpt","updated":true,
 "account":{"id":"acc-1","label":"work","email":"a@example.com",
            "account_id":"acc-1","plan":"plus","refreshable":true,
            "in_use":true}}
```

`updated:true` means this account already existed and was refreshed in place.

### `rm <target> <name>`

Move an account/slot to the trash. Recoverable only while a TUI session is
open (`z`); otherwise it is finalized when that process exits or on next start.
Do not use it to "test" anything.

```json
{"type":"rm","ok":true,"target":"zcode","trash":"1761…-work.json"}
```

### `import chatgpt <file|->`

Import Codex session JSON. Accepts one object, an array, concatenated
documents, or JSONL; also tolerates invalid trailing lines (`bad_docs`).
`-` reads stdin. Never auto-activates — call `use` afterwards.

```json
{"type":"import","ok":true,"target":"chatgpt",
 "imported":2,"skipped":0,"bad_docs":1}
```

Failure cases exit 1: no object with `access_token` (`found:0`), or all objects
unconvertible.

### `logout <target>`

Clear the local credential file so the app returns to a login screen. The
account stays saved; for chatgpt an unsaved live credential is saved first
(`auto_saved` in JSON). Does not revoke server tokens.

## Target keys and aliases

| Canonical | Aliases |
|---|---|
| `chatgpt` | `gpt`, `codex` |
| `zcode` | `z` |
| `antigravity` | `agy`, `ag`, `anti` |

Case-insensitive.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success (including an empty list) |
| 1 | operation failed — file missing/corrupt, account not found, app not logged in |
| 2 | usage mistake — unknown command/target/option, missing or extra argument |

## JSON error shape

```json
{"error":"在账号库中找不到 \"nope\"","ok":false,"type":"error"}
```

Note: with `--json`, errors go to **stdout** as JSON (so one parser handles
everything) and the exit code is non-zero.
