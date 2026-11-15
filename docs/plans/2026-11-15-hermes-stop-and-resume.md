# Hermes Stop: interrupt fence and what Stop records

Status: Proposed. The fence fix below has shipped; the Stop-ordering change waits on
stable resume identity (planned separately).

## The fence fix

Hermes refuses `session.interrupt` unless its `execution_generation` names the
generation running the turn. Crowbar echoed a value captured from the
`session.create` / `session.resume` / `prompt.submit` receipts, and every real
interrupt came back `stale_generation (4001)`. The type was not the problem: a
captured number was already sent back as a JSON number. The value came from the
wrong place, because no receipt carried the running turn's generation. Every event
frame does carry it, at the top level, as a number.

Now:

- `runtime.api.remember` keeps `execution_generation` from every event frame, and
  the interrupt sends `{captured.execution_generation}`. The receipt captures are
  gone.
- The engine stays generic. A value a `remember:` entry keeps is available to an
  outbound `send:` as `{captured.NAME}`, with its JSON type kept
  (`apidriver.Send`). The descriptor loader accepts `captured.NAME` only for names
  that some `remember:` entry captures.

Remaining window: if Stop lands after `prompt.submit` but before the new turn's
first frame, the interrupt names the previous generation. Hermes then refuses it as
stale, so nothing is cancelled by mistake, but the new turn is not stopped either.

## What Stop records before the provider agrees

`runner.stopRunner` commits `turn_ended {status: interrupted}` and a `stopped`
marker *before* the interrupt is sent. The interrupt runs in the background, and its
failure is only logged. When the interrupt fails, the ledger says the turn stopped
while the provider keeps running it:

- later `tool_call` / `assistant_text` rows land under a turn that is already closed;
- a prompt sent right after Stop is held by the provider until that turn finishes.

The fence fix removes the cause seen so far, but the ordering stays wrong for any
other refusal (transport loss, a provider that cannot cancel at that moment). The
fix is to make the provider's answer decide what is recorded:

1. Send the interrupt first and wait for its reply, bounded by the request context.
2. On success, record the stop as now. The provider's own end frame (`message.complete`
   with `status: interrupted`) closes the turn through `turn_stop`.
3. On failure, fall back to retiring the runner (`markEnding`), which really ends
   the provider's work, and record the stop then.

## Dependency: stable resume identity

Step 3 retires the runner on a failed interrupt. Today a retired Hermes chat comes
back through `session.resume`, and a session-resume identity bug can leave the
revived chat bound to the wrong session, or to none. So falling back to retire after
a failed interrupt would brick the chat. Until resume identity is stable, Stop keeps
its current order: the turn is recorded as stopped first, and a failed interrupt
leaves the provider running. The Stop-ordering change lands after the resume-identity
fix (separate plan).

## To verify live

The fence keeps the last `execution_generation` any event frame on the connection carried.
`remember:` runs on every frame before the ingress filters by session, and it keeps the
latest value, not the highest. Neither is a risk in the code or the recorded frames (each
runner's frames name only its own session, and generations only rise), but both depend on
gateway behaviour nobody has checked:

- whether one `gateway attach` connection ever carries frames of another session
  (a second chat on the same gateway, or a delegated child under its own session id);
- whether a late frame of an interrupted generation can arrive after the next
  generation's first frame;
- whether a delegated child's `agent.step` frames, which carry the parent's session id,
  move the parent's `step` capture.

If any of these happens, the generic guard is a `remember:` entry that only keeps a frame
whose session field equals the connection's session id.
