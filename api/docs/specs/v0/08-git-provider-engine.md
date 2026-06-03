# Crowbar Backend — Git Provider Engine

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`,
> `07-workspace-worktree-hierarchy.md`, `03-realtime-websockets.md`
> **Scope:** Read-only integration with GitHub and GitLab — protected-branch
> detection and pull-request state — to drive workspace `pr-*` badges and the
> locked flag. Covers the provider portions of UX spec §2, §11, §23.

---

## 1. Philosophy — Read-Only Observer

Crowbar **never writes** to the git provider. It does **not** create pull
requests — the user or an agent does that via `gh` / `glab` / the web UI.
Crowbar only **extracts** information:

1. **Which branches are protected** (→ sets the workspace `locked` flag).
2. **The PR state for a branch** (open / merged / closed + url + target) (→ sets
   the workspace `pr-*` status badge).

This is "an engine on itself," pluggable per provider, with **GitHub and GitLab
both supported at launch**.

---

## 2. Package Structure

```
internal/engine/provider/
  provider.go         GitProvider interface (read-only) + PRInfo, ProviderState
  internal/
    github/           via gh CLI (preferred) or GitHub REST API
    gitlab/           via glab CLI (preferred) or GitLab REST API
    detect/           which provider for a repo + what access is available
    poll/             scheduler: on-view + background sweep
```

```go
type GitProvider interface {
    ProtectedBranches(ctx, repo) ([]string, error)
    PullRequestForBranch(ctx, repo, branch) (*PRInfo, error)
}

type PRInfo struct {
    Number       int
    Status       string // "open" | "merged" | "closed"
    URL          string
    Title        string
    TargetBranch string
}
```

---

## 3. Access Detection & Enforcement

Provider capability **requires the provider's CLI** (`gh` for GitHub, `glab` for
GitLab). This is a deliberate decision: the CLI uses the user's existing auth,
needs zero configuration from Crowbar, and avoids us storing tokens.

`detect/` resolves, per repository:

1. The provider from the `origin` remote URL (github.com / gitlab.com / hosted).
2. Whether the matching CLI is installed and authenticated.

**Graceful degradation** (no REST-token fallback in v0):

- CLI present & authed → capability **enabled** for that provider.
- CLI absent / not authed / no hosted remote / offline → capability **disabled**:
  - PR state is **"unknown"** — workspaces simply never show `pr-*` badges.
  - Protected-branch detection falls back to a **config list**
    (`main`, `develop`, `master`, configurable).

The capability disabling is per-provider: on a machine with `gh` but not `glab`,
GitHub repos get live PR/protection data while GitLab repos fall back to config.

---

## 4. No Push — Polling Is Required

A local desktop tool **cannot receive provider webhooks** (no public endpoint,
behind NAT). There is also **no push/subscribe** for PR state in the CLIs: `gh`'s
only `--watch` is on `gh pr checks` (CI checks) and is itself interval polling
(default 10s). There is no event stream for "is this PR merged yet."

Therefore PR/branch-protection state is **polled**.

### Polling model (`poll/`)

- **On-view:** when a workspace row renders in the sidebar or its review panel
  opens, query that branch's PR state **once, immediately** — fresh data when the
  user is looking.
- **Background sweep:** a slow ticker (default **60s**, configurable) re-polls
  **only workspaces with an `open` PR**. `merged` / `closed` are terminal, so
  those workspaces drop out of the sweep. This bounds API usage to branches with
  live PRs.

Each poll that detects a change issues a `SyncProviderState` command to the
**Workspace** aggregate (§5).

> **Accepted deviation from UX §17.** UX §17 lists `pr-open/merged/closed` as
> "Event-driven." Because no provider push exists for a local tool, the real
> behavior is poll-driven: after a user merges a PR, the badge can lag up to one
> sweep interval (~60s), or updates immediately if the row/panel is viewed
> (on-view poll). This is a conscious trade-off, not an oversight.

---

## 5. Integration With the Workspace Aggregate

PR state is **not** a standalone aggregate (this revises
`00-architecture-and-domain.md` — see §6 there). The provider engine is just
another **producer** that issues commands to the Workspace aggregate:

```
poll detects change
  └─► SyncProviderState{ wsId, prInfo?, protected }  → Workspace aggregate
        └─► Workspace updates: status (pr-open|pr-merged|pr-closed),
            prUrl, prTitle, prTargetBranch, locked
              └─► Workspace Asynx subscription fires
                    └─► hub.BroadcastWorkspace(Workspace)  → Workspaces topic (global)
                          └─► sidebar badge + review panel update live
```

So provider sync rides the **same** Class A path as every other workspace state
change (`03-realtime-websockets.md`): command → event → subscription → hub →
broadcaster. No new broadcaster, no new WS endpoint.

> **`locked` can transition after creation.** It is resolved at creation, but a
> later poll may flip it — e.g. a workspace created while `gh` was unauthenticated
> (config-list fallback → `locked:false`) where the branch later proves protected
> once `gh` is available. If a workspace gains `locked:true` after the user has
> already made local commits/merges into it, the lock takes effect **going
> forward** (no further local mutations / merges-into); already-applied local work
> is **not** rewound. Integrating it then requires a provider PR (`07` §3.2).

Fields added to the Workspace aggregate by this engine:

```
status   ... | pr-open | pr-merged | pr-closed     (already in the enum)
locked   bool                                       (set from ProtectedBranches)
prUrl          string?
prTitle        string?
prTargetBranch string?
```

---

## 6. REST Surface

Read-only convenience routes (the live updates flow over the Workspaces WS):

```
GET /v0/workspaces/:wsId/provider     current PRInfo + protected flag (on-demand poll)
GET /v0/repos/:id/protected-branches  protected-branch list for the repo
```

`POST`-ing a PR is intentionally **absent** — Crowbar never creates PRs.

---

## 7. Out of Scope

- Creating / editing / merging PRs on the provider (never ours; user/agent does
  it). Crowbar reads state only.
- REST-token auth fallback (v0 enforces CLI presence; may revisit later).
- Webhooks (not viable for a local tool).
- Providers beyond GitHub + GitLab (extensible via the `GitProvider` interface).
