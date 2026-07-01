# Protected-Branch Provisioning & Recovery — Feature Spec

**Status:** Draft (design) · **Date:** 2026-07-01

> This is a **feature spec** — it describes *what* Crowbar should do and how it
> behaves for the user. Code-level detail (files, functions, signatures, tests)
> lives in the implementation plan, not here.

## 0. Scope

Making protected-branch import **robust and recoverable**, and giving the user a
**visible, actionable** surface whenever a protected branch can't be turned into
its own managed workspace.

Out of scope: the already-shipped provider auto-reparent guard (protected
branches are never reparented; only open PRs auto-reparent). Independent, done.

---

## 1. The problem

Two things go wrong today when importing a repo whose protected branches
(e.g. `develop`, `master`) are already checked out somewhere:

1. **A protected branch silently disappears.** If something already holds the
   branch — the imported folder itself, another worktree, or a leftover checkout
   from a previous import that was deleted from disk but not from git's records —
   Crowbar can't create its managed worktree, so it just **skips the branch with
   no message**. The user sees one branch where they expected two, and nothing
   tells them why.
2. **Crowbar silently moves the user's checkout.** When the imported folder sits
   on a protected branch, Crowbar force-detaches it to a headless state without
   asking, to free the branch for itself. Data is safe, but the user's working
   directory silently stops being "on" their branch.

The user hit both on `asynx`: after clearing Crowbar's data and re-importing,
only `master` came back, and the repo folder was left detached — with no
explanation for either.

---

## 2. Goals / Non-goals

**Goals**
- A protected branch is **never silently dropped**. It always becomes either a
  healthy managed workspace or a **visible placeholder** the user can act on.
- **One consistent way to free a held branch**, used everywhere Crowbar needs a
  branch: automatically reclaim dead leftovers, and *offer* to free live holders.
- The user's real checkout is **only moved with consent**.
- The user can **keep working** — branch off a protected branch — even while it's
  a placeholder.

**Non-goals**
- Creating or editing pull requests (Crowbar never does).
- Silently seizing a branch a user is actively using in their own worktree — we
  surface it and ask.
- Migrating old persisted state (pre-production; dev resets by clearing data).

---

## 3. The feature

### 3.1 Free the branch, don't drop it

When Crowbar needs a protected branch's own workspace and something already holds
that branch, it behaves by holder type:

- **A dead leftover** — a checkout whose folder was deleted but that git still
  records — is **reclaimed automatically**. This is the common case after
  clearing Crowbar's data, and it should "just work" with no user involvement.
- **A live holder** — the imported repo folder, or another worktree still on
  disk — is **never touched without consent**. Instead the branch becomes a
  placeholder (§3.2) that explains who's holding it and offers to free it (§3.6).
- **Already managed by Crowbar** — the branch already has a workspace; nothing to
  do.

The rule: reclaim what's safe to reclaim automatically; for anything that would
disturb the user, surface it and let them decide. Never skip in silence.

### 3.2 The placeholder workspace

A protected branch that couldn't get its workspace still **appears in the sidebar
as a branch row** — just in an error state:

- It has **no files, no git, no chat, no terminal** — it isn't a real worktree
  yet, so those surfaces are simply unavailable.
- Its row shows an **error/warning indicator** (not the normal lock icon).
- Clicking it opens an **error panel** explaining why it couldn't be set up (e.g.
  *"`develop` is checked out at `<path>` — free it so Crowbar can manage this
  branch"*) with two actions: **Retry** and **Free & manage** (§3.6).
- **Retry** re-attempts setup **for this same branch/row** — on success the row
  turns into the real, healthy protected workspace. It never spawns a duplicate.

### 3.3 Branching off a placeholder

Users often want to start work off a protected branch immediately, so a
placeholder is **not a dead end**:

- **Creating a child branch off a placeholder is allowed.** The child is based on
  the branch's last-known local state (which may be slightly behind the remote —
  the same tolerance Crowbar already applies when branching off a stale parent).
- **What's deferred:** merging that child *back into* the protected branch locally
  needs the branch's real workspace, so it waits until the placeholder is
  resolved (Retry). The pull-request flow is unaffected. Once resolved, the user
  can bring children up to date against the now-current branch.

So: branch and work now; local merge-back becomes available once the branch is
properly set up.

### 3.4 The repo home is just another holder

The imported repo folder is treated as one possible holder of a protected branch,
like any other — **no more silent force-detach**. If the folder sits on a
protected branch, that branch becomes a placeholder whose **Free & manage** action
moves the folder off the branch, **with the user's consent** via the modal
(§3.6). This replaces today's silent behavior: Crowbar no longer moves the user's
checkout without asking.

### 3.5 Surfacing — persistent and immediate

- **Persistent:** the placeholder row itself is the standing record — it stays in
  the sidebar until resolved, so the situation can't be missed after the moment
  of import.
- **Immediate:** when a protected branch first lands as a placeholder, Crowbar
  also shows a **toast** ("Couldn't set up `develop` — checked out elsewhere")
  with a **Fix…** action that opens the detach flow.

### 3.6 The "Free & manage" (detach) modal

Freeing a live holder is an explicit, consented action. Clicking **Free & manage**
opens a modal that:

- **Explains what will happen:** the checkout at `<path>` moves to a headless
  state, releasing `<branch>` so Crowbar can manage it.
- **Is framed as disruptive, not destructive:** files and commits are untouched;
  only which branch that folder is "on" changes. The copy must make the safety of
  the user's work clear.
- **Won't run mid-operation:** if the holder is in the middle of a merge, rebase,
  or similar, the action is unavailable, with an explanation, rather than failing
  halfway.
- On confirm: free the holder, then Retry the branch into a real workspace.

---

## 4. Resolved decisions

1. **Consent for all holders**, including the repo home — nothing that moves the
   user's checkout happens without the modal. Supersedes "the repo home is always
   detached off a protected branch."
2. Placeholder branches are shown in a distinct **"unprovisioned" error state**.
3. **Reclaim dead leftovers automatically** before setup, always — it only affects
   records whose on-disk folder is already gone, so it's always safe.

---

## 5. Acceptance criteria (what the user should observe)

- Import a repo with two protected branches → **both appear**, as managed
  workspaces or placeholders — never a silent single-branch result.
- Clear Crowbar's data and re-import → the previously-managed protected branches
  **come back automatically** (dead leftovers reclaimed), no user action needed.
- A protected branch held by a live worktree → shows as a **placeholder** naming
  the holder, with **Retry** and **Free & manage**; **Retry after freeing it**
  turns it into the real workspace.
- The imported folder on a protected branch → that branch is a placeholder;
  Crowbar **does not detach the folder until the user confirms** the modal.
- A placeholder branch → the user can **create a child branch** off it; local
  **merge-back is unavailable** (with a clear reason) until the placeholder is
  resolved.
- The detach modal states the operation is **safe for files/commits** and is
  **blocked mid-merge/rebase**.
- A newly-created placeholder also raises a **toast** with a Fix action.

---

## 6. Out of scope

The provider auto-reparent guard (issue #1, shipped). PR creation/editing.
Legacy state migration.

---

## 7. Rollout

Pre-production, no users: no migration. Existing dev state is reset by clearing
Crowbar's data. Additive behavior.
