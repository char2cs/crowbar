# Remote Instances — Design Spec

Status: LOCKED (approved 2026-09-27, in-conversation review — architecture, auth, and
all three UI screens each approved individually; see chat history for the
brainstorming record and the visual-companion mockups under
`.superpowers/brainstorm/`).

Supersedes `docs/superpowers/specs/2026-08-18-federated-backends-design.md`
(preserved, unbuilt, at `backup/server-side-companion-2026-09-27` — that
design's "Merged" cross-host tree mode is explicitly dropped, see §6).

## 1. Goal

A remote instance is a headless Crowbar daemon (`api/`) running on a machine
the user doesn't have open in front of them — first target: a Linux VM on
the user's Proxmox host. The desktop app can hold connections open to
several such daemons (plus the local one) at once, and every project from
every connected daemon appears in the sidebar's project/space selector as
if it all came from one daemon — no separate "remote" section, no merged
cross-host tree, just one flat list, tagged internally by which daemon owns
each project.

Non-goals: cross-host project merging, a "Merged" view mode, host-to-host
awareness of any kind, sync/replication, offline caching, multi-user/
permissions.

## 2. Component boundaries

**The Go daemon (`api/`) never learns about other daemons.** Its only new
responsibility is authenticating clients *to itself*:

- Reuses the self-signed TLS identity code already built and tested on
  `backup/server-side-companion-2026-09-27` (`tlsid` package) — ported over,
  adjusted for server-TLS-only (no client cert requirement, see §3).
- Adds a loopback-only pairing-code mint (CLI: `crowbar pair`), a
  network-reachable rate-limited redeem endpoint, and a device/token
  registry with **permanent** revocation (no re-pair reactivation of a
  revoked device — an explicit improvement over quiver's own behavior,
  where a fresh code reactivates a revoked device id).

**All multi-host knowledge lives in the desktop client**, in a new
`web/src/features/hosts/` module (not `features/connections/` —
`lib/ws/connection-store.ts` already owns that name for WS-transport
health, a different concept):

- A small persisted config list: `{id, name, endpoint, pinnedFingerprint}`
  per paired host; `id: 'local'` is always implicit and never removable.
  Bearer tokens live only in the OS keychain, keyed by host id — never in
  the persisted JSON.
- A registry mirroring `features/workspace/stores/workspace-store-registry.ts`'s
  Map-keyed pattern: one live runtime per host id (status, resolved
  API/WS base), independent lifecycle, genuinely concurrent — confirmed:
  the user wants real concurrent connections, not swap-on-navigate, though
  scope stays per-project (never two hosts backing one open pane/chat at
  once).
- `apiFetch`/`wsUrl`/`openSocket` (today: a single module-level `API_BASE`
  read from `window.__CROWBAR__.api`) change to take an explicit `hostId`,
  resolved from **the acting project's own** `hostId` field on its
  workspace store — never an ambient "currently viewed" host. This
  codebase already has a documented bug class from exactly this shape of
  mistake with workspace ids (an ambient-fallback prop used for a
  data-creating action silently wrote into the wrong workspace,
  `pane-container.tsx`, fixed 2026-09-15); the same discipline applies to
  host routing from day one.
- `useProjectDataStore` (today: `fetchProjects()` against the single
  `API_BASE`, live-merged over one `/v0/projects` WS) fetches from every
  `up` host and merges into the same flat, tagged list — one store,
  multi-sourced, which is what makes the sidebar "look like one daemon."
- A new Settings tab lists saved hosts with live status and revoke,
  alongside the add/pair dialog.

## 3. Auth & transport

Chosen over two alternatives (mirror quiver exactly with no transport
encryption; full mTLS with client certs) — **server-TLS + bearer token,
quiver's pairing-code UX**:

- Daemon terminates TLS with its self-signed identity (from `tlsid`); no
  client certificate is required. The credential is the bearer token, not
  a cert — closer to quiver's actual mental model, while closing the
  plaintext hole quiver itself leaves open (unacceptable for a daemon
  reachable over the open internet, which this one plausibly is).
- **CLI, on the daemon's own host only:** `crowbar pair --name <label>`
  mints a single-use, 6-digit, 5-minute-TTL pairing code and prints it
  alongside the daemon's TLS fingerprint. This is the *only* way to
  generate a code — matches quiver exactly, and matches the user's
  explicit choice: pairing a new client always requires CLI access to the
  host, with no in-app "invite another device" shortcut even from an
  already-trusted client.
- **Desktop, Settings → Hosts → Add host**, two-stage dialog (quiver's own
  shape):
  1. Name + address → client dials, completes the TLS handshake, shows the
     presented fingerprint for the user to confirm against what the CLI
     printed (trust-on-first-connect).
  2. 6-digit code entry → redeemed over that now-encrypted channel
     (rate-limited server-side) → daemon returns a bearer token once.
- **Persisted:** `{id, name, endpoint, pinnedFingerprint}` in the hosts
  config list; the raw token goes only to the OS keychain.
- **Every later call:** TLS handshake re-checked against the pinned
  fingerprint — a mismatch is a hard reject with a clear "this host's
  identity changed" warning, never a silent fall-through — plus the bearer
  token as `Authorization: Bearer`.
- **Health:** per-host probe against `/v0/health` (interval + on-demand)
  drives up/down/unreachable, surfaced in Settings.
- **Revoke:** Settings → Hosts → Revoke marks the device permanently
  revoked server-side and drops the local keychain entry/config row. A
  401 mid-session on an otherwise-"up" host marks it needs-repair rather
  than silently retrying — re-pairing still requires CLI access regardless.

## 4. Project aggregation, routing & down/up behavior

- Startup and on host add/remove: `useProjectDataStore` fetches
  `/v0/projects` from every `up` host, tags each project with its
  `hostId`, merges into the existing flat list. One WS subscription to
  `/v0/projects` per live host, frames merged the same way.
- Opening a project records that project's `hostId` on its workspace
  store at creation; every workspace-scoped call (git, terminal, LSP,
  chat, file tree) resolves its API/WS base from **that workspace's own**
  `hostId` — the concrete fix point for the ambient-fallback bug class
  above.
- **Host goes down** while its projects are listed: rows stay visible
  (nothing vanishes), but any action against them fails fast with a clear
  "host unreachable" error instead of hanging.
- **Host comes back up:** re-fetch its projects, resume its WS
  subscription, clear the error state — no app restart.
- **Host removed/revoked:** its projects drop out of the aggregated list
  via the existing deleted-project tombstone path (`status: 'deleted'`),
  same as an actually-deleted project.

## 5. UI

Approved via the visual-companion brainstorming session
(`.superpowers/brainstorm/*/content/`):

### 5.1 Project row

Two-line shape, same rhythm as `ChatBranchHeader`'s title/subline pair:

- Title in `var(--font-heading)` (Instrument Serif), same size class as
  today's single-line row.
- Subtitle below: a status dot (`--success` green when reachable, a
  muted grey when not) + the owning host's name — **always shown,
  including local, labeled "Local"** — so nothing reads as a special
  case; a project's remote-ness is never a distinct visual language, only
  the subtitle text differs.

### 5.2 Create a Space — daemon picker

A new "Daemon" field row inserted between the Space Name field and the
existing Folder field, same input-row visual language. A dropdown lists
Local (default) and every saved host with its live status dot;
unreachable hosts are listed but disabled (can't create a project you
can't reach yet) rather than hidden, plus a trailing "+ Add a host…"
entry that opens the Settings → Hosts add flow.

### 5.3 Remote file picker

Native OS picker stays for local hosts, unchanged. For a remote host, no
native dialog can browse a filesystem that isn't the user's own, so a
React fallback opens instead: **built on Coss UI's tree component**
(`coss.com/ui` — a shadcn-CLI-style registry on top of `@base-ui/react`,
already in `package.json`; copy-paste/owned the same way every other
`@/components/ui/*` primitive is, not a new opaque dependency), the
"tree with icons" variant, restricted to single-select with no
drag-drop/rename/checkboxes.

Explicitly **not** a reuse of `features/file-explorer/`'s
`FileExplorerTree` — that component is keyed to an already-open
`workspaceId` and carries git-status decoration, full CRUD, drag-and-drop,
inline rename, search, and chat-scoped live WS sync, none of which applies
before a project exists. The one thing carried over from it is the
*pattern*: lazy per-directory fetch (`fetchFileTree(wsId, path)`'s shape),
reused here against a new "list this directory on host X" endpoint instead
of a workspace's `/tree`.

Expand-in-place (not breadcrumb drill-down): folders expand lazily on
click (one directory-listing call per expand, never the whole tree up
front), files are shown but inert/dimmed, one folder selectable at a
time, a host badge + "Select" action in the footer.

## 6. Explicitly dropped from the prior (reset) design

The `2026-08-18-federated-backends-design.md` spec (unbuilt, preserved at
`backup/server-side-companion-2026-09-27`) proposed a second "Merged"
window mode: a cross-host unified repo view, a host-glyph column,
`(repo, branch)` uniqueness enforcement across hosts, and host-qualified
placeholders for a losing branch. **None of this is being built.** Per
the user directly: "Merged won't be a thing." The desktop instead holds
N concurrent connections and presents every project from every host in
one flat, undifferentiated list — see §4. This spec's project-tagging
model (§2, §4) replaces that design's row-level host glyph entirely.

## 7. Testing

- **Go:** table-driven tests for `tlsid` (server-TLS-only mode), the
  pairing usecase (mint/redeem/TTL/single-use/rate-limit/permanent-revoke),
  and the auth middleware (loopback bypass vs. TLS+bearer path) — one test
  file per source file, per `api/`'s conventions.
- **Web:** mirrored under `web/src/__tests__/features/hosts/...` —
  registry add/remove/status transitions, the pairing dialog's stage
  transitions and error states, and — highest value — a test asserting a
  project-scoped call resolves strictly from that project's `hostId` and
  never an ambient default.
- **E2E:** a Dockerized daemon (image already scaffolded by
  `13ce34de9 fix(docker): ship an image that can actually run the daemon`,
  preserved at `backup/server-side-companion-2026-09-27`) stands in as the
  "remote instance." Full flow driven live through `make dev-desktop` +
  the Tauri MCP bridge: pair via `crowbar pair` inside the container,
  add the host from Settings, confirm the fingerprint, redeem the code,
  confirm the host's projects appear in the sidebar tagged correctly,
  create a new project on the remote host via the daemon picker, browse
  its filesystem via the Coss UI tree picker, kill the container and
  confirm down/up behavior, revoke and confirm removal.
