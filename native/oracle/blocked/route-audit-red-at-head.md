# Blocked — `TestRouteAudit_AllSpecRoutesRegistered` is red on `rewrite/rust`

**Found:** 2026-07-30, during Phase 0 item 0.6 · **Not caused by the port.**
**Status:** needs a decision from whoever owns those two routes.
**Blocks nothing** — recorded so it is neither forgotten nor quietly absorbed.

## The failure

```
go test -tags 'integration noEmbed' -run TestRouteAudit_AllSpecRoutesRegistered ./internal/api/v0/

registered route not in spec/superset: POST /v0/projects/:projectId/repos/:repoId/workspaces/import
registered route not in spec/superset: GET  /v0/projects/:projectId/repos/:repoId/pull-requests
registered route count drifted from expected set: should have 159 item(s), but has 161
```

`api/internal/api/v0/route_audit_test.go:326,328`.

## It is genuinely pre-existing

I reproduced it **on a clean tree before merging anything**, on the merge base,
with no `native/` changes and no `/v0/settings/ui` in the router. Same two
routes, same 161-vs-159 count. This is not an attribution — it is a reproduction.

Two routes were added to the router without being added to the audit's spec
list. The count assertion is a hardcoded 159 that was never bumped.

## Why it is not merely someone else's problem

The audit exists to catch exactly one class of mistake: a route registered but
never declared. The Rust-native port **adds routes** — `/v0/settings/ui` in
0.6, and a loopback listener in 0.7 — and will add more.

While this gate is red, it cannot tell anyone whether a *new* undeclared route
has appeared; it just stays red and gets scrolled past. A permanently-failing
test is worse than no test, because it trains everyone to ignore the one signal
that would have caught the next instance.

For 0.6 specifically I could still read the answer out of the failure output —
`/v0/settings/ui` is **not** in the undeclared list, so 0.6's routes are
correctly declared. That worked because there were only two pre-existing
offenders. It does not scale.

## The fix, which is small

Either add both routes to the audit's spec/superset list and bump 159 → 161, or
delete them if they are dead. Then the gate is green and the port's later route
additions are actually checked.

**Not done here** because `api/` is out of scope for this port except for the
single §9.3 exception, and expanding that scope unasked is precisely what §0
forbids. It is a two-line change for whoever owns those routes.
