# `primitive-dialog-service` (P3.33) — scope ruling: covered by `dialog`, no new surface

> A §6.2 row, in the shape `native/MAPPING.md` fixes, except that this row's
> value is a ruling rather than a mapping: `web/src/components/ui/
> primitive-dialog-service.tsx` **ports to nothing**. No Rust file exists for
> it, and none should — see below.

## 0. The question this item asked before building anything

`primitive-dialog-service.tsx` (210 lines, 4 importers: `main.tsx`,
`features/tabs/components/tab-context-menu.tsx`,
`features/settings/components/tabs/developer-settings.tsx`,
`features/git/hooks/use-git-diff-handlers.ts`) exports `primitiveAlert`,
`primitiveConfirm`, `primitivePrompt` and a `PrimitiveDialogProvider`. It
looked, on the brief's own reading, like "a request queue plus a `Dialog`
render" — app behaviour rather than a new visual primitive. The brief's
instruction was explicit: **determine whether it needs a new surface at all,
and report before building one**, with "covered by `dialog`; no new surface"
named as a fine and expected outcome.

**It is that outcome.** Every box `primitive-dialog-service.tsx` paints is
`dialog`'s own box, under `dialog`'s own already-declared anchors, at
`dialog`'s own already-verified numbers. Nothing here needs a Rust file.

## 1. What the file actually renders

Read in full (`web/src/components/ui/primitive-dialog-service.tsx`, all 210
lines). The three exported functions (`primitiveAlert`/`primitiveConfirm`/
`primitivePrompt`) are pure promise-returning enqueue calls — no JSX, no
DOM, nothing an oracle could ever anchor. `PrimitiveDialogProvider` renders
`{children}` plus, when a request is queued, one `<DialogHost>`. `DialogHost`
is the whole of this file's visual surface:

```tsx
<Dialog open onOpenChange={(next) => !next && dismiss()}>
  <DialogContent>
    <DialogHeader>
      <DialogTitle>{request.title}</DialogTitle>
    </DialogHeader>

    {request.kind === 'prompt' ? (
      <form className="flex flex-col gap-2" onSubmit={...}>
        <label className="flex flex-col gap-2 text-sm text-foreground">
          {request.message}
          <Input autoFocus value={value} placeholder={request.placeholder} onChange={...} />
        </label>
      </form>
    ) : (
      <div className="whitespace-pre-wrap text-xs text-foreground">{request.message}</div>
    )}

    <DialogFooter>
      {request.kind !== 'alert' && <Button variant="outline" onClick={dismiss}>{request.cancelLabel}</Button>}
      <Button onClick={confirm}>{request.confirmLabel}</Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

Five imports, all already ported and verified: `Dialog`, `DialogContent`,
`DialogFooter`, `DialogHeader`, `DialogTitle` from `@/components/ui/dialog`
(P3.21, `dialog` — **PASS, 0 deltas over 4 anchors**, re-verified on the
merged tip per `native/QUEUE.md`'s Wave 5 sweep), `Button` (P2.1) and `Input`
(P2.11). No new element, no new class list, no new token this file
introduces on its own account.

## 2. This is exactly the shape `dialog.md` §3 already names and disposes of

`native/mapping/dialog.md` §3, "The body is a *height*, and the description
is modelled-but-unreached":

> the space between the header and the footer is a call site's own content —
> a two-field form, a settings panel, a confirmation prompt — and the port
> takes its measured extent (172px at the reachable call sites) rather than
> reproducing one of them.

`DialogHost`'s body — the `<form>`/`<label>`/`<Input>` for a prompt, or the
plain `<div>` for an alert/confirm — **is** "a confirmation prompt", the
literal example `dialog.md` already used to describe what a call site's body
looks like. It is not a new visual primitive; it is one more instance of the
same "call site's own content between `dialog-header` and `dialog-footer`"
shape `dialog.rs`'s `Dialog::body_height` already models as a plain measured
extent, alongside `add-repository-modal`'s and `import-project-modal`'s own
two-field forms. Porting `DialogHost`'s body as its own surface would be
porting the *content* `dialog`'s own module docs already say is out of
scope — "none of that is this primitive, and a port that reproduced one call
site's contents would be measuring that call site," `popover`'s docs put it,
and `dialog.md` repeats the same call for its own body.

`DialogFooter`'s two buttons (`Button variant="outline"` / plain `Button`)
are call-site content in exactly the sense `dialog.md` §5 already covers:
`import-project-modal`'s and `add-repository-modal`'s own footers each carry
two `Button`s too, and neither is separately anchored — the footer's own
bounds are what `dialog-footer` reports, and the buttons inside it are
`button`'s surface, not a new one this file would need to declare.

## 3. The four importers all reach the same two anchors `dialog.md` already has

Every one of `PrimitiveDialogProvider`'s four live mount points
(`main.tsx` — the provider itself; `tab-context-menu.tsx`'s
`primitiveConfirm` on a destructive tab-close; `developer-settings.tsx`'s
`primitiveConfirm`/`primitivePrompt` on a few dev-only actions;
`use-git-diff-handlers.ts`'s `primitiveConfirm`/`primitivePrompt` on
destructive git actions) renders through the identical `DialogHost` markup
above. None of them adds a class, a wrapper element or a `data-oracle-id`
of its own — there is nothing per-call-site to distinguish, unlike `dialog`
itself, whose 11 live `<Dialog>`/`<AppDialog>` call sites genuinely vary
(`sm:max-w-md` vs `h-[70vh] max-w-md`, a description present or absent). Every
`DialogHost` cell is `DialogContent` with no extra class, a header, an
unmodelled body and a footer — precisely the two already-verified reachable
cells `dialog.md` measured (`add-repository-modal`, `import-project-modal`),
modulo the body content dialog already treats as unmodelled.

## 4. The ruling

**Covered by `dialog`; no new surface.** `crates/crowbar-ui/src/components/
dialog.rs`'s existing `ID_POPUP`/`ID_HEADER`/`ID_TITLE`/`ID_FOOTER` anchors,
`Dialog::body_height` and the two Button/Input primitives it composes with
already describe every box `primitive-dialog-service.tsx` paints. Nothing is
ported here:

* No `crates/crowbar-ui/src/components/primitive_dialog_service.rs`.
* No `crates/crowbar-app/src/{surfaces,row_layout}/primitive_dialog_service.rs`.
* No `data-oracle-*` edit to `primitive-dialog-service.tsx` — it has nothing
  this port's own anchors would need beyond what `dialog.tsx`'s existing
  `data-oracle-id`s already carry (`DialogHost` renders those same
  components, unmodified).
* No `/tmp/p3-ref-primitive-dialog-service*.json` — there is no anchor set
  distinct from `dialog`'s own `/tmp/p3-ref-dialog.json` to capture a
  reference for.

This is the "covered by `dialog`, no new surface" outcome the brief named as
fine and expected, applied. It is a **narrower** finding than the five
components already recorded as "unreachable-and-ported-blind"
(`native/QUEUE.md`'s Wave 5 sweep: `sheet`, `radio-group`, `toast`,
`textarea`, `select`) — those five have their own Rust surface, built and
tested without a live reference because none was reachable. This item found
no surface to build at all: building one "just in case" would be the mistake
the brief calls out as worse than porting blind, because it invents a thing
to verify that the evidence says is not there.

## 5. What would change this ruling

If a future call site gave `DialogHost` its own class, its own extra
wrapper, or a body shape `dialog.md`'s existing unmodelled-body treatment
does not cover (e.g. a fixed viewport-relative height the way
`repo-import-dialog.tsx`'s undriven `h-[70vh]` popup would need, per
`dialog.md` §5), that cell would need its own capture and its own line in
`dialog.md`'s own reachability table — still not a new surface, because the
primitive underneath would still be `dialog`'s. Only a `DialogHost` shape
that painted a box `DialogPopup`/`DialogHeader`/`DialogTitle`/`DialogFooter`
does not already anchor would ever warrant a new file, and nothing in the
four live call sites read this pass does.
