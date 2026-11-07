# Release safety: install, upgrade and rollback on real homes

Status: audited 2026-11-07 on Linux with the embedded production binary, isolated
`CROWBAR_HOME`s and copies only. macOS was read, not run. Two open items below need
work before a home much larger than the ones measured here is upgraded.

## Matrix

| Scenario | Result | Evidence |
|---|---|---|
| Fresh home, first boot (no providers on PATH, space and non-ASCII in path, `LANG=tr_TR.UTF-8`) | ok: all steps run, home 0700, files 0600 | live boot, turns, attachments |
| Develop home (51 chats, 20 worktrees, 197 turns, 5 attachments, settings, 2 descriptor overrides) → this build | ok: same chats, titles and providers; 1884 ledger rows from 1273 legacy records, 0 skipped; attachments served; history renders in the browser | API diff, ledger counts, screenshot |
| Second boot: `retire-legacy` | ok: 202 files deleted only after a completed layout-1 boot, each byte-checked against the backup; view and ledger unchanged | ledger + API diff |
| kill -9 at 37 points (every 75 ms across backup, steps 2, 6, 8, 9, 12, 1, 7) | ok: every restart resumed; view and ledger identical to a clean run; no halt | sweep log |
| Disk full after the backup, mid `chat-ledger` | ok: halts "the disk holding Crowbar's data is full" + backup path; resumes once space is freed; identical result | tmpfs in Docker |
| Not enough room for the backup | ok: halts in preflight before any change | tmpfs in Docker |
| Newer `LAYOUT`, unreadable `LAYOUT`, corrupt ledger line | ok: halt page; nothing written but `state/.lock` | tree diff |
| Read-only home; home that cannot be created | ok: halt naming the file (fixed); exit with the reason (usage text removed, fixed) | live |
| A file another user owns in the home | was a permanent halt at step 11; fixed: left as is and logged | Docker as root wrote the file |
| Two daemons on one home (this build twice; this build + develop) | ok: the second refuses, home locked | live |
| Develop crashed with live runners, then this build | ok: no orphan CLIs; turns resume | live |
| Pre-ledger seeds (big 55k records, small, corrupt, clean, empty) | ok | live |
| Moved repo, deleted worktree, worktree without `.git` | ok: boots, all chats listed | live |
| Rollback: develop on a migrated, retired home | as documented: chats listed, history empty; nothing corrupted | API |
| Restore from the newest backup (README steps), then develop, then this build | ok: develop sees the exact pre-upgrade history; re-upgrade gives the identical ledger | API + ledger diff |
| Develop turns added after the upgrade, then this build again | kept in the legacy files (retire keeps changed files) but not shown | ledger query |
| 500 chats / 112k legacy records | 40 s in step 6, peak RSS 310 MB, progress shown | /proc sampling |
| 500 chats / 443k legacy records | 2.5 min in step 6, peak RSS 1.0 GB | /proc sampling |
| Old develop tab left open across the upgrade | stuck on "Backend unavailable" until reloaded | screenshot |
| Runner tokens | argv clean; `mcp.token`, `hook.curl`, `settings.json` 0600 in 0700 dirs, removed on stop; not in logs | live |

## Open: `chat-ledger` holds the whole legacy history in memory

`ReadLegacy` loads every legacy chat before converting any (sorting by age needs
them all), so peak memory grows with total history: about 2.3 KB per legacy record
measured (310 MB at 112k, 1.0 GB at 443k). A home with a few million records would
need several GB during the first boot.

Plan (owner: `adapter/migrations/chatledger`): read the chat ids with their oldest
timestamp first (one query per legacy table; add an index on `chat_id` in a scratch copy if the per-chat reads need it), then read, convert and import
one chat at a time. The order, checkpoints and notes stay as they are; only the read
becomes per chat. Pin it with a test that converts N chats while asserting at most
one chat's records are held. Until then, measure the owner's own home with
`crowbar migrate --plan` and the record counts before release.

## Open: no way to tell a stale bundle from the daemon

The daemon reports `version: 0.1.0` for every build, so neither a tab left open
across an upgrade nor the desktop shell (which adopts any daemon already answering
on its socket, logging only a pid mismatch) can tell it is talking to another build.
Plan: stamp a build id into the binary and the web bundle, return it on
`/v0/health`; the web reloads once on a mismatch, and the desktop shell refuses
(and names) a daemon of another build instead of adopting it. A develop tab cannot
be fixed after the fact: the release notes say to reload.

## Residual risks, ranked

1. Memory of the first upgrade on very large homes (above).
2. Version skew in an open tab or with a stale desktop daemon (above).
3. Turns made by an older build after the upgrade are not shown (kept on disk).
4. The restore does not move attachments or `.refused` overrides back; the README
   now says so.
5. Step 11 makes the scaffolding above checkouts 0700: another local user or a
   container uid can no longer reach a worktree through the home.
6. A unix socket path over 108 bytes (long `TMPDIR` or `HOME`) fails to bind, with
   the path in the error.

Not run: macOS (paths, keychain, sidecar: read only; no updater plugin is
configured), Windows (unsupported), the owner's real home, a home with no `git`
anywhere (the login-shell PATH always found one).
