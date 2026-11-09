# Migration evidence: legacy homes to the chat ledger

Status: measured 2026-11-08 on Linux, `-tags noEmbed` builds, every daemon in a
network-less container with the home at its original path (or an isolated
`CROWBAR_HOME`), fake provider CLIs only. Evidence lives under
`$T/tmp/d-migrate/` (`$T` = the SDD tools dir); paths below are relative to it.

## Legacy shapes

Seven on-disk shapes, found by diffing the schemas each historical build leaves
(`hist/shapes.txt`):

| Shape | Builds | What it lacks against the next |
|---|---|---|
| S0 | 0ccaeda0 | no activity tables at all |
| S1 | 10e6f8e3 | turn status/diff/plan, tool kind/locations/diff/subagent, subagent messages, conversation last activity; folders without `home_id`; pre-#179 chat folders |
| S2 | 49399113 | as S1 plus subagent/messages/last activity |
| S3 | 9ccd87db | + placements, `nodes_read`, view tables |
| S4 | f39f2599 | + turn status/diff/plan, tool kind/locations/diff |
| S5 | 728d8478 | + runners with `reported_*` |
| S6 | cc28ad7a, 59388771, 898e9f05 | the owner's home (fingerprint `9a4dc0a7bb`) |

## Matrix

| Scenario | Result | Evidence | Residual risk |
|---|---|---|---|
| S6 homes driven by 3 builds (34 chats, 160 prompts, 415 tools, 31 subagents, 19 asks, 6 attachments, folders, renames, deletes) → tip | identical semantic view, two boots, attachments byte-equal | `hist/<c>/cmp1.txt`, `cmp2.txt` | 2 chats show a prompt the old build left `uncertain` (now a user row + `prompt_settled{consumed:false}`) |
| S1–S3 homes → tip | **was a halt** at step 6 (`no such column: status`) and step 7 (`no such column: home_id`); fixed | `hist/retip.log`, `hist/retip2.log` | pre-#179 chat folders (`agent_chat_folders`) are not migrated, as develop already dropped them at 9ccd87db; their chats list at top level |
| S0, S4, S5 homes → tip | boots; lists, folders, attachments match | `hist/retip*.log` | those builds recorded no transcripts with the fake |
| Chained: S1/S2/S3/S5 → 59388771/9ccd87db → tip; S6 → 898e9f05 → tip | ledger rows identical to the direct upgrade (minted owning chats aside) | `chain/all.log` | — |
| Owner's real home (copy): 21 chats, 3351 records, 8 attachments | tip direct and via 59388771: identical ledgers; 6439 rows, 0 skipped; every one of 1094 tools with the same refs; 392 subagents; harness prompts kept as harness | `real-copy/runs/*`, `real-copy/runs/oldthentip2/out/cmp-b.json` | see "Owner's home" |
| Fuzz, 200 seeds, 4–14 of 19 damage kinds each, on an S6 home | 200/200 boot; 196 equivalent with every diff inside a damaged record's chat, 4 also serve a chat the old build answered 500 for; 0 halts; 200/200 backups hold `state/` byte for byte | `fuzz/summary.txt`, `fuzz/s<n>/` | every skip is a record the seed damaged (see "Fuzz skips") |
| Scale: 20,024 chats, 3,257,853 records (2.5 GiB), 140,162 prompt journals, one 5,040-turn chat | completed: 5,895,652 rows, 0 skipped; gate answers 503 in <1 ms throughout; live heap flat (2–4 MB over 3,000 chats; was 2→30 MB) | `scale/clean.log`, `scale/resume.log`, `scale/gctrace*.log`, `scale/runs/clean/root/samples-*.txt` | see "Scale" |
| Scale: SIGTERM at 7,410 s mid step 6 (17,561 chats done), then an abrupt exit (SIGQUIT) mid rescan, then resume | resumes at chat 17,562 and completes; every chat checkpointed once | `scale/resume.log`, `scale/evidence/resume-stuck-in-ids-goroutines.out` | rescan before the first new chat: 25 min cold |
| kill -9, 34 points across backup and every step, owner's copy | 34/34 resume to the identical ledger | `real-copy/sweep.log` | — |
| Oldest home (S1, 10e6f8e3): kill -9 at 17 points 100–6,500 ms, SIGTERM at 5 | 22/22 resume to the clean run's ledger | `crash/10e6f8e3.log` | — |
| Oldest home with transcripts (S6, cc28ad7a, 1,779 rows): kill -9 at 26 points 200–9,000 ms, SIGTERM at 5 (3 inside step 6) | 31/31 resume to the identical ledger | `crash/cc28ad7a.log` | — |
| Two daemons on one home | second exits `another crowbar instance is using this home`; first completes identically | `crash/*/out/second.out` | — |
| Read-only home | halts `io_failed` "data folder is read-only", serves the halt page; no file changed (sha256 of every file) | `crash/*.log` "read-only" | — |
| Disk full before the backup (2, 5, 20 MB free; and 1 MB more than the backup needs) | halts `preflight_failed` naming bytes needed and free; with the needed space it completes; freed, the same home boots to the identical ledger | `crash/*.log` "disk full" | — |
| Disk fills right after the backup (tmpfs) | halts `step_failed` "disk … is full (your data … is backed up in <path>)" at step 6 or 7; once at step 3 as `verify_failed` "18 replayable chats without a type remain"; twice the migration finished and the boot exited `hooks.url.tmp: no space left on device`; every case, freed, gives the identical ledger | `crash/fullmid*.log` | a disk-full in step 3 reads as a verification failure, not as disk full |
| Backup → restore (README command) → owner's build → tip, owner's copy | owner's build sees the pre-upgrade view (boot-stamped times aside); tip re-upgrade gives the identical ledger | `real-copy/restore.log` | attachments stay at `chats/<id>/attachments`; the older build does not find them until moved back (README says so) |

## Defects fixed

1. A home an older build last wrote halted step 6 on a missing column
   (`0e6fe479`): later-added columns read as NULL; any other missing column halts.
2. 139 of the owner's subagents were skipped: their records kept the pending id
   `turn-<chat>-<seq>` an old reply had before its text (`bcf37e63`).
3. A home from before home folders halted step 7 on `home_id` (`174579eb`).
4. Step 6 kept every imported chat's fold in memory (`81a7bed0`), and its
   post-import check folded the chat again through the same cache
   (`b61c70b9d`): RSS grew ~19 KB a chat (210 MB at 6k chats, 440 MB at 17.5k
   with only the first half of the fix); with both the live heap stays 2–4 MB.

## Owner's home

Counts only: 21 chats (5 with transcripts), 1626 turns, 1094 tools (16 error,
4 abandoned), 392 subagents (4 never ended), 159 interruptions, 3 choices,
67 prompt journals (all accepted), 2106 tool refs with 0 missing blobs, one
live runner row (the copy was taken live). Anomalies: 141 subagents under a
pending or open reply id (fixed above); 18 placement rows of deleted chats
(dropped as orphans, as designed).

Differences the owner will see, none a loss:
- 83 of 655 replies in one chat, and 7 in another, are dated later than the old
  build showed them (never past the reply's own end): a reply that was still
  streaming when an injected harness message arrived sorts after it, as the old
  build's own display order put it. 2 pairs of adjacent replies swap the same way.
- 236 of 1094 tools are drawn after such a harness message (up to 82 min after
  they ran), inside the reply that holds them.
- 397 older tools the old build's 500-call activity page never served are shown.

## Fuzz skips

Each seed copies the S6 home and applies 4–14 damages: wrong role, corrupt or
non-JSON columns, NULLs, unicode, empty strings, bad and skewed times, dangling
turn ids, pending reply ids, duplicate ids, deleted chat rows, orphan chats,
damaged journals, missing later-added columns, an old schema, a huge row, damaged
conversations, providers, titles and parents. Every skipped record names the
damage that made it: unknown role, unreadable journal or `requestId`, a
subagent/tool whose turn the damage removed, an unparseable column (locations,
plan, messages, options), an unparseable time, an unknown interruption kind. No
skip names an undamaged record (`fuzz/summary.txt` lists each reason with the
seeds' damage kinds).

## Scale

Host: ZFS over a NAS, I/O pressure 35–45 % from the other runs.

- Backup: 2.9 GB of `state/` → 433 MB tar.gz in 98 s.
- Step 6: 2.4 chats/s contended (17,561 in 7,297 s), 6 chats/s alone; fsync-bound.
  Written: 7.9 GB for 2.6 GB legacy (≈3×, backup included); `chat_ledger.db`
  2.53 GB against 2.60 GB of legacy activity and runner databases (0.97×).
- Peak RSS 253 MB on the resumed boot (SQLite page caches and the chat-id index);
  flat during conversion.
- Step 7 (placement tree): 32 s for 3,034 chats, 19 min for 20,034. Each owning
  chat it places republishes its workspace, which lists every chat; the
  generator copied owning chats (5,022 owners in 22 workspaces, real homes have
  one per workspace), so this is O(workspaces × chats) on a real home.
- Peak RSS for a whole run of the 3,034-chat home: 72 MB.
- First open after the boot: sidebar 175 s to its first frame, 1.8 s after; the
  5,040-turn chat (23,769 rows) 0.33 s. The daemon then holds 887 MB.
- Second boot: retire-legacy checks the backup byte for byte and deletes 2.75 GB
  of legacy files: 24 min behind the migrating page.

## Residual risks, ranked

1. **First sidebar open on a big home is O(all ledger rows).** The projects
   snapshot folds every chat's ledger (`Snapshots.preloadLedger` →
   `Store.View`): 175 s for 20k chats here, and every fold stays in
   `Store.writers`, never evicted (887 MB). Not migration code; a sidebar
   projection is a second copy of state and needs its own plan. The owner's
   21-chat home opens in well under a second.
2. **Long, silent waits on a big home.** Health reports only `step 6, index 1`;
   a resume first rescans every legacy row and 140k journal files (25 min cold
   here) and step 7 costs O(workspaces × chats), both without visible progress.
3. **The second boot deletes legacy files after a full backup check**: 24 min
   for 2.5 GB here. Safe, but another long migrating page.
4. Reply and tool order around injected harness messages (owner's home: 90
   replies dated later within their own lifetime, 236 tools drawn after such a
   message). Nothing lost.
5. A disk that fills inside step 3 halts as `verify_failed`, not as disk full;
   the step logs each failed write as a skipped row. Data and backup intact.
6. Pre-#179 chat folders (`agent_chat_folders`) are not migrated, as on
   develop since 9ccd87db.
7. After a restore, attachments stay under `chats/<id>/attachments`; the older
   build looks elsewhere until they are moved back (the README says so).
