# codex fixtures

`<method with / replaced by _>.json` is a LIVE CAPTURE off a real codex
app-server connection.

`<same>.<variant>.json` is a SUM-TYPE VARIANT of the same wire method. A wire
method that serves several canonical events (codex's `item/started` and
`item/completed` are both `ThreadItem` sum types) needs one fixture per variant,
or the replay harness silently checks only whichever variant happened to be
captured — which is exactly how `tool_result: item.content` survived: the only
`item/started` recording was a `userMessage`, so `tool_pre`/`tool_post` were
never replayed against anything.

Files whose variant name ends in `-schema` are **derived from codex's own
generated protocol schema**, not captured live:

    codex-rs/app-server-protocol/schema/json/ServerNotification.json  (ThreadItem)

They are exact against that schema's field names and enum values, but they are
NOT proof of what a live codex emits — this repo's history already records four
paths written from a published schema that turned out wrong against real
traffic. Re-capture them live and drop the `-schema` suffix when a codex CLI is
at hand.
