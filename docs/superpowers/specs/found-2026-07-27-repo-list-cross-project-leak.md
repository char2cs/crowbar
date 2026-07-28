# Found (not fixed): `GET /projects/:projectId/repos` leaks every project's repos

Date: 2026-07-27
Found during: diff-subsystem-at-scale Phase 1, while importing a perf fixture.
Status: **recorded, deliberately not fixed** — out of scope for that program,
which is a performance change. Wants its own fix + regression test.

## The bug

`api/internal/api/v0/endpoints/repos/handlers/repos.go:263`:

```go
repos = filterByProject(repos, c.Query("projectId"))
```

The route is `/v0/projects/:projectId/repos` — the project is a **path**
parameter. The handler filters on the **query string** instead. No caller sends
`?projectId=`, so `filterByProject` receives `""` and returns `FindAll` intact.

## Reproduction

Two projects exist. Ask for one project's repos and get the other's:

```
$ curl --unix-socket <sock> http://localhost/v0/projects/6fa8b545-.../repos
  9bed4c1c-... detach-demo    <- belongs to a DIFFERENT project
  9bed4c1c-... athas          <- belongs to a DIFFERENT project
  6fa8b545-... big-diff
```

The returned DTOs carry the *other* project's `projectId`, so the response is
self-evidently unscoped rather than mislabelled.

## Why the existing guard does not catch it

`scopeRepoToPath` (`api/internal/api/v0/middleware.go:31`) enforces
`:repoId ⊂ :projectId`, but returns early when `:repoId` is empty — and its own
doc comment says so: *"Routes with no :repoId (the repo collection list/create)
pass through."* The collection list is exactly the uncovered case.

## Impact

Cross-project information disclosure: the repo list for project A includes
project B's repo names and **absolute filesystem paths**. In this session that
surfaced a user's real `~/Projects/Cloned/*` paths under an unrelated temp-dir
project.

Whether the UI happens to filter client-side is irrelevant — the daemon serves
the data to any caller of the endpoint.

## Suggested fix

Change `c.Query("projectId")` to `c.Param("projectId")`. Then add a black-box
`TestRegression_*` in `api/tests` that creates two projects with one repo each
and asserts each project's list contains only its own — the per-repo
`projectId` in the response makes that assertion trivial.

Worth checking the sibling collection endpoints for the same query-vs-param
confusion before closing it out.
