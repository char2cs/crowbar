# Cold start: a chat opens from its cached feed before any daemon lists

`AgentChatPaneGate` used to mount the chat body only once the project list named the chat's host,
so the IndexedDB feed cache (`chat-feeds`) was never read until the daemon answered, and with the
daemon unreachable never at all.

## What is built

- `host-placements` store (IndexedDB v16, `web/src/lib/persistence/host-placements.ts`): one row per
  `(hostId, kind, id)` for every workspace and chat a daemon lists. Written by a subscriber on
  `entityStore` (added rows put, removed rows deleted); read into memory in `hydrateCriticalStores`
  before the first render. The daemon stays the owner: when a host's projects scope first loads, the
  rows it no longer lists are deleted, so the store is a cache that its list replaces.
- `useChatPaneHost` (`web/src/lib/host-resolve-cached.ts`), used only by the gate: the resolved host,
  else the host that held the workspace (or chat) last run, while that host is configured and has not
  listed yet. Once it lists, its rows alone decide: same host keeps the body mounted, a different
  host remounts it, an absent row shows the existing "Chat unavailable" state.
- Every call that creates data still resolves through `hostIdForWorkspace`/`hostIdForChat`, which
  ignore the cache and refuse until a list names the host; sends while a host is unreachable take the
  existing `offline` path.
- A removed host's rows go with it (`onHostRemoved`), beside its feed entries. IndexedDB unreadable:
  hydration reads as empty and the gate waits as before.
