# P3.56 — Liveness audit of the 48 ported surfaces

Audit only. No code, mapping, or oracle files were modified; this file is the
only change. Method: for each of the 48 names in
`Surface::names()` (`native/crates/crowbar-app/src/surface.rs`), resolve its
React original (from the surface module's own doc comment and/or
`native/mapping/<name>.md`), then trace importers of that original from the
app root (`web/src/main.tsx` -> router -> routes -> ...), checking **both**
`@/components/...` and relative (`./`, `../`) import spellings at every hop.
A component is LIVE only if some import chain reaches a JSX call site that
actually renders on a path reachable from the root with no dead condition in
between (or a route/flag names a real gate, in which case CONDITIONAL).

## Control

**tooltip** — `web/src/main.tsx` imports `TooltipProvider` from
`@/components/ui/tooltip` directly and wraps `<RouterProvider>` in it (line 7,
100-107). This is as live as anything in the app: every route renders inside
it. Confirms the method reports a known-live component as LIVE. See tooltip's
row below for the full picture (Tooltip/TooltipTrigger content usage too).

## Headline

(filled in as rows complete; final tally at the bottom)

## Rows

| Surface | React original | Verdict | Evidence |
|---|---|---|---|
