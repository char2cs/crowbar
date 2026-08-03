# Blocked — 4 of 48 ported surfaces measure code no user can reach

**Raised:** 2026-08-03, by the liveness audit (P3.56, `native/mapping/liveness-audit.md`).
**Status:** needs a user product decision. **Nothing is deleted pending it.**

The user's directive was *"only port components that ARE IN USE on the production
app."* By then it had already been violated four times. This records what was
found, why the four are not the same kind of thing, and what each choice costs.

## They split into two kinds, and the split is the decision

### A. No render path at all — `toast`, `sheet`

| surface | evidence |
|---|---|
| `toast` | ports `ui/toast.tsx`'s `AnchoredToasts`. Its manager `anchoredToastManager` has **zero `.add()` calls** in `web/src`. Every real toast goes to the sibling `toastManager`, consumed by `sidebar-toast-overlay.tsx`'s `SidebarToastItem` — **which has no Rust port**. `native/mapping/toast.md` already said this before the surface merged. |
| `sheet` | its only consumer is `sidebar.tsx`'s `Sidebar` (`isMobile` branch), and a repo-wide grep for `<Sidebar` returns **zero** JSX renders. |

These are dead components. Porting them was wasted effort and keeping them
overstates coverage — **48 surfaces is really 46**.

### B. Live components on a branch that cannot fire — `skeleton`, `inline-error`

| surface | evidence |
|---|---|
| `skeleton` | its host is live — `<Suspense fallback={<SidebarSkeleton/>}>` at `sidebar-carousel.tsx:131` — but **nothing in the wrapped subtree ever suspends** (no `React.lazy`, no suspending hook), so the fallback is unreachable by construction. |
| `inline-error` | sole call site guards `wsListData.status === 'error' && repos.length === 0` (`workspace-tree.tsx:57`). The fetch chain bottoms out in `getAllEntities`, which is a bare `catch { return [] }` (`lib/persistence/entity-cache.ts:34`) — **it swallows every failure**, so `status` cannot become `'error'` from that source. |

**These are not dead code in the same sense.** The component exists, the call site
exists, the author intended it to render — and something upstream makes it
impossible. **That is arguably a defect in the React app**, not in the port: an
error state a user should see and cannot, and a loading state that never appears.

## The decision I am not making alone

1. **Delete the four ports?** Recovers nothing already spent and removes verified
   work. `toast` and `sheet` are clear candidates. `skeleton` and `inline-error`
   are not — if the upstream defect is fixed, those surfaces become live and
   correct, and deleting them means porting them twice.
2. **Or keep them and mark them?** Then the coverage number needs an asterisk and
   §17.9's *"a user cannot tell the apps apart"* is trivially satisfied for four
   surfaces by neither app ever showing them.
3. **And separately — are the two class-B cases bugs worth filing against the
   React app?** `getAllEntities` swallowing IndexedDB failures means the workspace
   list silently shows empty instead of erroring. That is a product question well
   outside this port's scope, and it is the user's call whether it matters.

## What is NOT in question

The other 44 are fine: **30 LIVE, 14 CONDITIONAL** — the conditionals each sit
behind a named route, flag or toggle (`fps-overlay`'s Developer-tab switch, the
`git` tab's home-route filter), which is a **cell axis the port must model**, not
a reason to skip. That distinction was itself a finding: I would have collapsed
CONDITIONAL into DEAD and been wrong about fourteen surfaces.

## Method note, because it nearly went wrong

The audit ran a **control** — `tooltip`, which `main.tsx` wraps around
`<RouterProvider>` — so a method that reported everything dead would have been
caught immediately. It also re-verified its own sub-agents rather than trusting
their summaries, and that caught a guard misquoted as `||` which is really `&&`.
My own first liveness scan reported a false *"0 importers"* for fifteen files by
missing relative sibling imports. **Any liveness claim without a control is
worthless**; that is now the standing rule.
