# Pane breakpoints follow the window, not the pane

## Problem

Develop renders the whole app in one webview, so a Tailwind breakpoint (`sm:` 640px, `md:` 768px,
`lg:` 1024px, `xl:` 1280px) is a question about the window. In the shell, each pane is its own
webview, so the same media query asks about the pane. The IDE editor pane is usually under 640px
wide, so every `sm:` rule in it is off:
- ED-035 / EDSIZE: `Button`'s `text-base sm:text-sm` loses to the chip's `ui-text-xs`, giving
  11px text where develop shows 14px.
- Any `md:`/`lg:` layout in a narrow pane collapses to its mobile form.

## Owner

The shell owns the window's inner width, the same way it owns the theme. It derives from that
width the set of breakpoints the window clears, and pushes the set to panes as a read-only input,
the way `ShellFrame::Theme` already works (`panehost/src/pane_host/context/broadcast.rs`). It
sends on mount, and again only when the set changes, so a resize stays quiet between thresholds.
No pane writes it back, and nothing else keeps a copy that anyone can edit.

## Steps

1. **Protocol.** Add `ShellFrame::Breakpoints { cleared: Vec<Breakpoint> }`, where
   `Breakpoint` is `Sm | Md | Lg | Xl` at Tailwind's 640/768/1024/1280 px, and
   `Breakpoint::cleared_by(width)` derives the set. Round-trip it in `pane_frame`'s table test.
2. **panehost.** Add `PaneHost::set_breakpoints(cleared)`, a twin of `set_theme`: unchanged sets
   send nothing, and it sends to mounted panes and again on `mount`. The shell calls it from the
   window's bounds observer that already lays out panes. Tests: a width change that crosses no
   threshold sends nothing, and one that crosses a threshold reaches every mounted pane once.
3. **SDK.** Add `PaneClient.onBreakpoints(listener)`, plus `pane-boot` code that sets the root
   attribute `data-window` to the pushed set (`"sm md lg"`) before the first render and on each
   push. The pane's own `innerWidth` stays as it is, because positioning code
   (`use-dropdown-position.ts`, the file-explorer context menu, `use-pane-top-row-edges.ts`)
   rightly measures the pane.
4. **CSS (panes only).** In `app/panes/src/index.css`, redefine `sm`/`md`/`lg`/`xl` (and
   `max-sm` …) as `@custom-variant sm (&:where(:root[data-window~="sm"] *, :root[data-window~="sm"]))`
   and likewise for the others. Tailwind 4.3 lets `@custom-variant` replace a built-in variant
   (`dark` is the documented case). The first commit proves it with a red test: a built
   stylesheet in which `sm:text-sm` applies under `data-window="sm"` and not without it.
5. **JS media queries.** The audit of `app/panes/src` found no width `matchMedia`, no
   `useMediaQuery` and no width-based `innerWidth` branching; every `matchMedia` is
   `prefers-color-scheme`, `prefers-reduced-motion` or `resolution`, which are window-independent.
   A lint rule (`no-restricted-syntax` on `matchMedia` with a `width` query, its message
   pointing at this plan) keeps it that way.
   Develop's web code is untouched.
6. **Rig.** Pair ED-035 at the standard size: the toolbar chip at 14px in a 540px editor pane.

## Not in scope

- Container queries for panes that should follow their own width. None exist in develop.
- Mobile (Tauri), which still renders one webview and keeps real media queries. It gets no
  `data-window` attribute, so the custom variants must fall back to the media query when the
  attribute is absent. Step 4's selector does that with
  `@media (width >= 40rem) { &:where(:root:not([data-window]) *) }` beside the attribute form.
