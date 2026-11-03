# Skipped legacy chats: making them visible

## Problem

Step 6 (`chat-ledger`) can skip a legacy chat for good. That happens when:

- nothing of the chat can be read,
- its records do not convert into a chat at rest,
- its import failed at `importAttempts` boots, or
- the import committed but did not land as converted.

The chat keeps its sidebar entry and its legacy files, but nothing in the
UI says that its history was not imported. Today the only traces are a WARN
or ERROR line in the daemon log and a `skipped/<chat>` note in
`migrate/ledger.jsonl`.

A skipped chat that already holds rows of its own (`ErrChatNotEmpty`) is
handled in this change: a `legacy_unreadable` marker is appended after its
rows and shown as a pill. That works because the chat can never be imported
into again, so a row costs nothing.

## Why a skipped empty chat gets no row

The ledger only imports into an empty chat (`Store.Import` refuses one with
rows), and the ledger is append-only, so a row cannot be removed later.
Writing a notice row into a skipped empty chat would therefore block the one
recovery it has: the import by a newer converter, which retries every chat
an older converter skipped.

## Who owns the signal

- The run ledger owns which chats were skipped, and why: the `skipped/<chat>`
  notes with their `converter`.
- The chat read model (the chat snapshot the web folds) is what the user
  sees. It reads only the chat ledger, never the run ledger.

The signal therefore has to cross from the run ledger to a read model, as
derived state rather than a second copy of the truth.

## Plan

1. A read-only query over the run ledger: the chats skipped by the current
   converter, each with its plain-words reason. It is computed from the
   notes at boot and cached for the daemon's life, because the notes change
   only at boot.
2. Show it on the chat: the chat snapshot carries
   `legacy: {notImported: "<reason>"}` when the chat is in that set. The
   transcript draws it as an empty-state notice ("This chat's history from
   before the upgrade could not be imported; the old daemon's files still
   hold it"). Nothing is written to the chat's ledger, so a newer converter
   can still import.
3. Show it in health: `/v0/health` reports `legacyChatsNotImported: <n>` next
   to `chatDeadLetters`, so support can see the count without the log.

## Tests

- Go: a skipped empty chat's snapshot names the reason. After a converter
  bump imports it, the snapshot no longer does.
- Web: an empty chat with `legacy.notImported` draws the notice, and a chat
  without it draws nothing.
