# /resume moves the conversation (#233)

Status: done.

**Goal:** A `/resume` typed in a chat's CLI that picks a session another chat
owns never leaves the CLI running unrecorded. Exactly one chat holds a
provider session at any time.

## Problem

A chat whose conversation was lost is relaunched on a fresh, unturned
session. The user types `/resume` there and picks a session chat A owns.
`claimSession` (`adapter/store/ledger/store.go`) refuses the binding with
`ErrSessionClaimed`, `ledger.Announce` records only `session_conflict`, and
every later hook of that CLI names a session that is not the chat's bound
one. Ingress dead-letters them (`namesAnotherConversation` →
`ErrForeignConversation`), so the chat silently stops recording while its
CLI keeps running.

## Owner of the state

The ledger owns session → chat: the `chat_ledger_sessions` index, written
only in the same transaction as the `session_bound` row that claims it.
The fix changes no copy of that state. It only adds rows that move the claim.

## Decisions

**D1. A chat that holds nothing takes the session.** If the announcing
chat's current conversation is not resumable (unbound, lost, or bound but
never turned: a stand-in, a fresh or a just-assigned id), `Announce` given
the owner returns a `session_bound` row with cause `moved` and `from` =
the owner chat. No `session_changed` marker: an unturned session held
nothing.

**D2. The owner gives it up first.** Before the moved row is committed on
the new chat, the owner chat gets `ledger.MoveAway`: if the session is its
current bound one, a `session_lost` row (`movedTo` = new chat, reason
"resumed in another chat") plus prompt settles for the runner on it, as for
any loss. The chat then shows the existing session-lost footer, and its
next send starts fresh. The owner's live runner on that session is marked
ending (`moved`) and ended outside the chat's lock, so the two CLIs never
share the session. A session the owner already moved off is past history
there: it gets no row, only the claim moves.

**D3. The index follows both rows.** In `commit`:
- a `session_lost` with `movedTo` repoints the claim from this chat to
  `movedTo`;
- a `session_bound` with cause `moved` repoints it from `from` to this chat,
  or takes it if no chat holds it. A claim held by a third chat is still
  `ErrSessionClaimed`.
- During a commit, a plain binding of a session the same batch later moves
  away is not claimed, so an import replays in any chat order.

Losing first means a crash between the two commits leaves no chat holding
the session (the index names the new chat, which is not bound). The CLI's
next announcement then binds it as an ordinary claim. Two chats never hold
it at once.

**D4. A chat that holds a conversation keeps it, and its CLI ends.** If the
announcing chat's current conversation turned, the claim stays where it
is. The chat records the `session_conflict` marker, worded in the web as
"Another chat holds this session; its CLI was ended so nothing goes
unrecorded". Its runner is marked ending (`displaced`) and ended outside
the lock. The chat keeps its own binding, so the next send resumes the
chat's own session. No runner is left running unrecorded.

## Tests

- domain: `Announce` with an owner and an unturned/turned current
  conversation; `MoveAway` for the bound, past and unknown session; Check
  accepts cause `moved`.
- store: the moved row transfers the claim; a third chat's claim is
  refused; imports of both chats replay in either order.
- usecase: develop's `TestRegression_ResumeInARelaunchedChat_BindsTheConversationInPlace`
  ported (A's runner retired, A lost, B bound to `sA`, B's later hooks
  recorded); the conflict branch ends the CLI.
- black box (`api/tests`, fake CLI) if the harness can drive a session_start
  carrying another session id.
