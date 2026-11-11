# Release safety: install, upgrade and rollback on real homes

Status: audited 2026-11-07 on Linux with the embedded production binary, isolated
`CROWBAR_HOME`s and copies only. macOS was read, not run. First-upgrade memory, turns an older build adds after
the upgrade, worktree access for other uids and long socket paths are fixed; version
skew was dropped by the owner. The residual risks are ranked below.

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
| Develop turns added after the upgrade, then this build again | was kept but not shown; fixed: imported after the chat's ledger, once | ledger + API diff |
| Develop on a retired home (legacy files recreated), then this build | fixed: every develop turn and prompt imported, no duplicate, seqs contiguous | ledger query |
| kill -9 across the re-import (100–1500 ms) | ok: every run ends with the same 2282 rows | row hash |
| Another uid reading a worktree through the home | was denied after step 11; fixed, and repaired on homes narrowed earlier | Docker as uid 1001 |
| Home path long enough that `crowbar.sock` exceeds sun_path | was "bind: invalid argument"; fixed: short socket, CLI finds it | live |
| 500 chats / 112k legacy records | 40 s in step 6, peak RSS 310 MB, progress shown | /proc sampling |
| 500 chats / 443k legacy records | was 1.0 GB peak, ready in 114 s; now 81 MB peak, ready in 125 s, same ledger | /proc sampling, row diff |
| Old develop tab left open across the upgrade | stuck on "Backend unavailable" until reloaded (a develop bundle cannot be fixed) | screenshot |
| Runner tokens | argv clean; `mcp.token`, `hook.curl`, `settings.json` 0600 in 0700 dirs, removed on stop; not in logs | live |

## Fixed: `chat-ledger` held the whole legacy history in memory

`ReadLegacy` loaded every legacy chat before converting any, about 2.3 KB per
record. The step now lists the chat ids oldest first in one streaming pass, then
reads, converts and imports one chat at a time (rows found by their
`<chat id>\x00<id>` key when every row is keyed so, else by `chat_id`). Checkpoints
and order are unchanged; the 37-point kill sweep and a row-by-row ledger diff on the
443k home match the old build, except `runner_exited` times, which are stamped at
boot either way.

## Fixed: turns an older build added after the upgrade were hidden

Running develop on an upgraded home writes new turns into the legacy files (or
recreates them after `retire-legacy`). The next boot of this build used to note the
change and show nothing. Step 6 now re-reads such a chat and imports only the rows
beyond what its ledger already holds: the longest end of the ledger that matches
the start of the conversion gives the rows after it, or, when every legacy row is
newer and none is in the ledger yet, all of them. The rows keep their own times and
are appended atomically at the head the step read. A chat the newer build has
written to since, or one develop deleted, keeps its ledger and is noted once.
Verified with the develop binary: added turns, a chat created on develop, a chat
deleted on develop, a retired-then-recreated home, and a kill -9 sweep.

## Fixed: owner-only locked other uids out of worktrees

Step 11 made every directory above a checkout 0700, and opening the home narrowed
the root again at each boot. Step 11 now narrows only Crowbar's own state and leaves
checkouts and the directories on the way to them as they were; a worktree add gives
that way traverse (`--x`) only; step 13 `worktree-reach` gives traverse back once on
a home an earlier build narrowed. `chats/` and `state/` stay owner-only.

## Fixed: a socket path too long to bind

A home whose `crowbar.sock` path is over 107 bytes (103 on macOS) now binds
`$XDG_RUNTIME_DIR/crowbar-<hash>.sock`, or `/tmp/crowbar-<uid>/crowbar-<hash>.sock`
in a 0700 directory. The CLI, the desktop shell (through `crowbar paths`) and MCP
callbacks resolve the same path from the home. The daemon refuses a socket directory
another user owns or may write to.

## Dropped: version skew

The owner dropped the stale-bundle check (no build id, no shell change). A tab left
open across an upgrade must be reloaded by hand.

## Sizing a home before the upgrade

`scripts/legacy-size.sh [home]` prints the chats, legacy records (as step 6 counts
them) and bytes, opening each database read-only. On the 443k-record home step 6
peaks at 81 MB.

## Residual risks, ranked

1. A chat both builds wrote to after the upgrade keeps this build's history; the
   older build's turns stay in its files (the daemon log names the chat; the restore
   README says so).
2. A tab left open across the upgrade must be reloaded by hand (release notes).
3. The restore does not move attachments or `.refused` overrides back; the README
   says so.
4. `chat-ledger` on the 443k home is ready in 125 s against 114 s before the
   streaming change (one run each; memory went from 1.0 GB to 81 MB).

Not run: macOS (paths, keychain, sidecar: read only; no updater plugin is
configured), Windows (unsupported), the owner's real home, a home with no `git`
anywhere (the login-shell PATH always found one).
