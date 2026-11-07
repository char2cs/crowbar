# Release safety: install, upgrade and rollback on real homes

Status: audited 2026-11-07 on Linux with the embedded production binary, isolated
`CROWBAR_HOME`s and copies only. macOS was read, not run. The two serious open items
(first-upgrade memory, version skew) are fixed; the residual risks are ranked below.

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
| 500 chats / 443k legacy records | was 1.0 GB peak, ready in 114 s; now 81 MB peak, ready in 125 s, same ledger | /proc sampling, row diff |
| Old develop tab left open across the upgrade | stuck on "Backend unavailable" until reloaded (a develop bundle cannot be fixed) | screenshot |
| Tab of this build left open across a later upgrade | offers Reload on reconnect; reloads only when asked | Playwright |
| Runner tokens | argv clean; `mcp.token`, `hook.curl`, `settings.json` 0600 in 0700 dirs, removed on stop; not in logs | live |

## Fixed: `chat-ledger` held the whole legacy history in memory

`ReadLegacy` loaded every legacy chat before converting any, about 2.3 KB per
record. The step now lists the chat ids oldest first in one streaming pass, then
reads, converts and imports one chat at a time (rows found by their
`<chat id>\x00<id>` key when every row is keyed so, else by `chat_id`). Checkpoints
and order are unchanged; the 37-point kill sweep and a row-by-row ledger diff on the
443k home match the old build, except `runner_exited` times, which are stamped at
boot either way.

## Fixed: no way to tell a stale bundle from the daemon

`/v0/health` (served or behind the boot gate) now carries `build`: the commit from
the Go VCS stamp, `dev` for a binary built outside a plain checkout (a git worktree
counts). The web bundle already carries its short commit. A page the daemon served
compares the two on its first and every resumed socket and, on a mismatch, pins a
"Crowbar was updated" toast with Reload; it never reloads by itself, because an
unsent prompt lives only in the composer. `index.html` is served `no-cache`. The
desktop shell stamps its own commit and refuses a daemon it did not spawn whose
build differs (or which names none), showing why on the halt screen; `dev` on
either side is not compared.

## Residual risks, ranked

1. Turns made by an older build after the upgrade are not shown. Running develop on
   an upgraded home before `retire-legacy` writes new turns into the legacy files;
   the next boot of this build imports chats it never converted, but for a
   converted chat it only notes the change (`changed/<chat>`), since its ledger may
   have grown since and an incremental import would interleave two histories. The
   files are kept (retire keeps changed files), so nothing is lost, only hidden; the
   restore README says so. Not fixed: no cheap, safe merge exists.
2. A develop tab left open across the upgrade must be reloaded by hand (release
   notes); builds from this one on offer the reload.
3. The first-run screen shows no toasts, so a tab left on it across an upgrade gets
   no reload offer (it has nothing to lose; the next navigation loads the new page).
4. The skew check compares only stamped builds: `dev` builds (worktrees, `go run`)
   are never refused or offered a reload.
5. The restore does not move attachments or `.refused` overrides back; the README
   now says so.
6. Step 11 makes the scaffolding above checkouts 0700: another local user or a
   container uid can no longer reach a worktree through the home.
7. A unix socket path over 108 bytes (long `TMPDIR` or `HOME`) fails to bind, with
   the path in the error.

Not run: macOS (paths, keychain, sidecar: read only; no updater plugin is
configured), Windows (unsupported), the owner's real home, a home with no `git`
anywhere (the login-shell PATH always found one).
