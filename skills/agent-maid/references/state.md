# State layout, environment, and repair

## Where data lives

agent-maid owns one config directory and otherwise only touches the three apps'
own credential files.

```
$AGENT_MAID_DIR (default: ~/.config/agent-maid, Windows: %AppData%\agent-maid)
├── settings.json                 # TUI wizard result + UI prefs
├── chatgpt/store.json            # ChatGPT account library
├── chatgpt/trash/                # deleted accounts awaiting final purge
├── zcode/slots/<name>.json       # ZCode credential slots
├── zcode/trash/
├── antigravity/slots/<name>.json # agy token slots
└── antigravity/trash/
```

Credential files it reads/writes (never created from scratch except by a
switch):

```
~/.codex/auth.json                                          # chatgpt
~/.zcode/v2/credentials.json                                # zcode
~/.gemini/antigravity-cli/antigravity-oauth-token           # antigravity (agy)
```

All of these are `0600`. Slot files contain **plaintext credentials** (the agy
token, or ZCode's still-encrypted values verbatim) — treat the config directory
like a password store.

## Environment overrides

| Variable | Effect |
|---|---|
| `AGENT_MAID_DIR` | Config root (account library + slots) |
| `CODEX_HOME` | Codex data dir (default `~/.codex`) |
| `ZCODE_V2_DIR` | ZCode data dir (default `~/.zcode/v2`) |
| `ZCODE_ACC_SWITCH_DIR` | ZCode slot dir (default `<config>/zcode`); also disables legacy migration |
| `ZCODE_CREDENTIAL_SECRET` | Must match the value ZCode itself runs with, or saved slots become undecryptable |
| `ANTIGRAVITY_GEMINI_DIR` | Gemini data root (default `~/.gemini`) |
| `ANTIGRAVITY_CLI_DIR` | agy data dir (default `<gemini>/antigravity-cli`) |
| `ANTIGRAVITY_STORE_DIR` | agy slot dir |

These make it safe to point agent-maid at a temp directory for experiments —
prefer that over mutating the user's real accounts when testing.

## Per-target mechanics worth knowing

### chatgpt

- Library keyed by ChatGPT `account_id`; the list marks the account whose
  `account_id` matches the live `auth.json`, not merely the last activated.
- `use` merges into the existing `auth.json`: unknown keys and their order are
  preserved, `OPENAI_API_KEY` is cleared, a Codex-parseable placeholder
  `id_token` is synthesized when the source had none. Atomic write, `0600`.
- `import` understands WebSession JSON (`/api/auth/session`), sub2api/CPA
  exports, bare account objects, arrays, JSONL, and space-separated documents
  (which is what pasting multi-line JSONL into the TUI input produces).

### zcode

- Slots store `credentials.json` verbatim (still encrypted by ZCode). Labels
  are decrypted in memory only, using `ZCODE_CREDENTIAL_SECRET` or the
  platform fallback.
- A slot saved on another machine/key gets `⚠跨机/跨密钥` in the TUI and a
  `warning` field in `list --json`; setting the same secret on both sides fixes
  it.
- Switching while ZCode runs is risky (it rewrites credentials on exit). The
  CLI switches immediately and reports `zcode_running`; the TUI can instead
  wait for exit (`--wait-apply`).

### antigravity (agy)

- The credential is a single compact JSON file: `{token:{access_token,
  token_type, refresh_token, expiry, expires_in}, auth_method, id_token_email}`.
- agent-maid writes it back **byte-compatible with agy** — compact, no trailing
  newline, `0600` — so a restored slot is indistinguishable from a fresh login.
- Email comes from `id_token_email`, falling back to the `id_token` JWT claim.
- `logout` renames to `.antigravity-oauth-token.logged-out-<timestamp>`, exactly
  like agy's own logout, so it is recoverable.

## Repair recipes

**"It says logged in but the app asks me to log in again"** — the app is
running with credentials from before the switch. Restart it. Check
`list --json` → `running`.

**"slot can't be decrypted" / `decrypt_ok:false`** — `ZCODE_CREDENTIAL_SECRET`
differs from the one used when the slot was saved. Run agent-maid with the same
secret ZCode uses.

**"credential expired and can't renew"** — `refreshable:false`. Nothing local
can fix it; the user must log in again, then `save` to refresh the slot.

**TUI shows garbled symbols** — first-run wizard mis-detected the terminal.
Re-run it from the settings screen, or delete `settings.json` to be asked again.

**Leftover `auto-*` slots after switching** — expected: the outgoing account
was unsaved. Rename them (`agent-maid` TUI → `r`) or `rm` them once the user
confirms which account they belong to.

## What agent-maid will not do

- No network calls, no token refresh, no server-side revocation.
- No importing ZCode/agy accounts from files (no such format) — only `save`
  from a live login.
- No cross-machine sync of slots; move the config directory yourself and respect
  the key/secret caveats above.
