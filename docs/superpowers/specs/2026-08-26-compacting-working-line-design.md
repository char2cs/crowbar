# Compacting: bring the working line back, with a fixed verb

**Date:** 2026-08-26

**Status:** design, prototyped in the chat-state-lab review artifact
(https://claude.ai/code/artifact/8b011a0a-cd20-4c4a-9836-0f1e195ac98f)

## Three things the lab had wrong about "Compacting" before this pass

Re-verifying this state against real source turned up three inaccuracies in the lab itself, not
in the shipped app — worth recording since two of them look exactly like the kind of fabrication
this whole exercise exists to catch, just self-inflicted this time:

1. **A fabricated assistant message.** The lab showed "Context is getting long — compacting
   before I continue." as something the assistant said. Grepped `web/src/features/agent` for
   anything resembling it — nothing. Compaction has no announcing chat message, real or dead.
2. **Wrong placeholder copy.** The lab said "Compacting the conversation…". The real copy, in
   `agent-composer.tsx:91`, is `'Compacting… your message will be queued'`.
3. **Wrong composer availability.** The lab disabled the composer (`mode:'off'`). Real:
   `composer-state.ts`'s `acceptsTyping()` explicitly includes `'compacting'` — it's one of only
   two states where the field still takes typed input, because "Compaction does not take the box
   away — it queues what is typed into it, which is exactly what a busy turn does"
   (`agent-composer.tsx:85-88`). Fixed to match: real placeholder, and `wireDockField()` wired so
   the send button toggles normally instead of staying permanently off.

## What's actually proposed here

`WorkingLine` (`working-line.tsx`) is real, and it really does go silent during compaction — its
own `blocked` check (`pendingChoices(activity).length > 0 || blockedOn(activity) !== null`) hides
it for any open interruption, compaction included, per its comment: "A CHAT WAITING ON A PERSON IS
NOT WORKING." Compaction isn't waiting on a person, but it's swept into the same check because
`blockedOn` doesn't distinguish "blocked on a human" from "blocked on the CLI's own housekeeping."

**Proposed:** reuse `WorkingLine`'s exact treatment (flicker spinner, elapsed clock) during
compaction instead of showing nothing, with one change — the verb is fixed at "Compacting", not
drawn from the rotating flavor-verb list (`Bootstrapping`, `Reticulating`, ...). Those exist
because Crowbar genuinely doesn't know what the agent is doing at any given instant; compaction is
the opposite case — a specific, known operation — so treating it like unlabeled busywork would be
a regression in honesty, not just a missed detail.

No tool list under it: compaction isn't a tool call, so there's nothing to enumerate.

Prototype: `workingLine('Compacting', 0, [])` + `startCompactingClock()` (elapsed-clock ticking
only, no verb rotation — a copy of `startWorking()`'s clock half) in the lab artifact's
`id === "compacting"` render case.
