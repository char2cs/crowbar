# The sidebar and pane surface — one row, one drag, one sidebar

**Date:** 2026-08-28

**Status:** design spec, **closed**. This is the **frontend** spec. A companion
session takes it from here and writes the backend implementation spec; the two
are then built together. Nothing here is built in the product yet.

**This is a rewiring and a restyling, not a manufacture from zero.** Crowbar
already has a working row system, tab bar, pane container, drop zones, carousel
and card. Every surface below is described in terms of those components and
what changes about how they are *wired*. Do not rebuild a row; reuse
`workspace-row-base` / `ROW_BASE` and give it different buttons. The prototype
this spec was drawn in is a **description of behaviour, not a source of
components** — nothing in it is to be ported.

**No migration, and no legacy path.** Write this as if Crowbar were about to
ship for the first time and is installed nowhere. No backfill, no compatibility
shim, no dual-read. Graceful fallback only, where a fallback is free.

**Desktop only.** Touch is out of scope; no gesture below needs a touch
equivalent.

**Baseline.** [`2026-08-23-unified-sidebar-design.md`](./2026-08-23-unified-sidebar-design.md)
is the model half of this work — four row kinds, two facts, the placement tree,
the verbs and the wire contract. It assumes
[`2026-08-23-agents-chat-rearchitecture-design.md`](./2026-08-23-agents-chat-rearchitecture-design.md)
has landed. **This document is the surface half**: what the sidebar and the
content area look like, what every control does, and what may never happen. It
revises no decision in either baseline.

**Prototype.** Every rule below was drawn, driven and measured in a live
prototype before being written down. Where a rule says "verified", it was
exercised against the running artboard, not reasoned about.

**Not in scope:** migration, the `engine/agents` work, the descriptor, and the
protocol. `feature/chat-wrapping` lands first; this extends what it leaves.

---

## 1. The laws

Nine rules. Everything else in this document is a consequence of one of them,
and any change that breaks one is a change to the design, not to the code.

1. **ONE SIDEBAR.** There is never a second rail, a second tree, or a second
   scroll region beside the first. The primary display is a vertical monitor;
   two rails do not fit on one and never will.
2. **A ROW IS A ROW.** One `.row` component, one drag, one hit test. Where a row
   was picked up decides nothing about what can be done with it. Rows differ
   only in which *buttons* they carry.
3. **A PANE HOLDS EXACTLY ONE CHAT.** The tab concept, as a way to hold several
   chats in one frame, is gone. A pane is a slot; what is open *inside* a chat
   belongs to the chat.
4. **A CHAT IS IN AT MOST ONE PANE.** Two panes on one chat share one chat's
   state, so the second is not a second pane, it is a mirror, and every switch
   looks like it fired twice.
5. **THE CHAT IS DURABLE, ITS VIEW IS NOT.** Closing anything ends a *view*.
   Only deletion removes a chat.
6. **EVERY DROP ADDS.** No gesture silently evicts. The only thing that removes
   a pane is closing it.
7. **A CHAT IS LISTED ONCE.** Whatever goes up leaves every arrangement that was
   remembering it.
8. **NOTHING RE-SORTS ITSELF.** Order in Recents is the user's; only a drag
   changes it. State changes never move a row.
9. **ANYTHING RUNNING HAS A ROW.** A chat the daemon is working on is visible
   whether or not a pane holds it.
10. **REUSE, DO NOT MANUFACTURE.** Every surface here is an existing Crowbar
    component, rewired and restyled. A new component is a last resort and needs
    a reason written down.

---

## 2. Shape of the whole thing

Top to bottom, the rail holds exactly this:

```
┌ rail ─────────────────────────────┐
│ window chrome  (traffic lights,   │  flex: none
│   sidebar toggle, back/fwd/prefs) │
├───────────────────────────────────┤
│ SPACES  (one panel per project,   │  flex: 1, x-snap scroller
│          horizontal snap)         │
│   ┌ space ────────────────────┐   │
│   │ space header (project)    │   │  flex: none
│   │ ┌ scroller ─────────────┐ │   │  flex: 1, y-scroll — ONE region
│   │ │ tree rows             │ │   │
│   │ │ ─── Recents ───────── │ │   │
│   │ │ recents rows          │ │   │
│   │ └───────────────────────┘ │   │
│   └───────────────────────────┘   │
├───────────────────────────────────┤
│ FILE EXPLORER CARD  (floats,      │  absolute, bottom: 8px
│   Files ⇄ Git carousel)           │  ALWAYS THE LAST ELEMENT
└───────────────────────────────────┘
```

**The file explorer card is always the last element of the sidebar.** Nothing
goes below it.

**The tree and Recents are one scrolling group**, not two stacked panels. One
gesture moves the whole rail. The cost is stated and accepted: with a long tree,
Recents is below the fold until you scroll.

There is **no tab bar** above the rail and **no context pill**. The tree is not
one view among four any more — it *is* the rail, and the row you are in is the
"you are here". There is **no "New Project" row** at the foot: a project is a
space, not an item in someone else's list.

---

## 3. The row

One component. Kebab-case file, `.row` class, `ROW_BASE` geometry: `h-9`,
`px-1.5`, `mx-1.5`, `my-0.5`, `gap-1.5`, 13px label. Right inset is 10px rather
than 6px, because the trailing control sat tight against the row's own border;
the leading glyph keeps its 6px.

### 3.1 Anatomy

```
[ glyph ] [ label (+ optional sub-line) ]   [ trash ] [ + ] [ chevron ]
```

- **glyph** — says what the row *runs in*: a git mark owns a worktree, a chat
  bubble borrows its parent's. This is the only signal of ownership and it is
  load-bearing; two sibling rows under a locked branch are otherwise identical
  while one writes into the parent's checkout and the other into its own.
- **label** — the chat's name. A branch name is a git ref wherever it appears,
  including on container rows whose name *is* one (`name-mono`).
- **trash** — see §9. Leads the trailing cluster.
- **+** — one create control per row; the mark says what it makes. On a row
  whose parent is git-capable it makes a **workspace**; on a row that is not, it
  makes a **thread** (elbow mark). Never both.
- **chevron** — folds. Containers are always expandable: a container can always
  be given something.

Every hand-rolled mark on a row — the chevron, the `+`, the trash — is
**`size-3` (12px)**, and the leading glyph is 16px (20px for a project). A
chevron drawn at glyph size reads as an illustration; it is a control.

The trailing action recipe is `ROW_SUB_ACTION`, verbatim: `inline-flex shrink-0
rounded-lg p-1.5 text-muted-foreground hover:bg-sidebar-element-hover
hover:text-foreground`. `rounded-lg` is `--radius` (10px); the 6px of
`--radius-sm` belongs to the project header's icon buttons, not to a control on
a row. Trailing controls are revealed on row hover.

### 3.2 States

| state | treatment |
|---|---|
| resting | transparent, `--fg` label |
| working | the glyph becomes the **flip-dot spinner** in place, not beside |
| hover | `--accent` |
| open (has a view) | **greyed label**, mark at full strength |
| focused | *(nothing in the tree — see below)* |

**A row with a view is grey — all of them, the focused one included.** The
focused row used to keep a raised background and a second line with its branch;
that made the tree answer "which am I looking at" as well, in a different shape,
for the same chat. Two rows for one chat have to be the same row. **Focus is a
Recents idea; the tree only says what is up.**

The mark keeps full strength either way: a working chat must not read as stalled
just because it is already open.

### 3.3 The second line, and why it is gone

The row began as two lines — `ICON | CHAT NAME` over `branch/name +128 -2k` —
shown **only on the active row**, so the tree kept its density. That survived
until Recents existed. Once the band answers "which am I looking at", a tree row
that also answered it was answering twice, in a different shape, for the same
chat. **No tree row goes to two lines any more.** The branch, the diff counts
and the ownership question are answered by the glyph and by the card.

### 3.4 Provisional naming

A new workspace is minted with a generated slug that is **both the title and the
branch name**. Both are marked provisional (italic, on both lines). They settle
at different moments: the title once, early, when the agent calls
`set_chat_title`; the branch when the task is achieved and the agent renames it.
Branch rename happens in git, not on the directory — Crowbar worktree
directories are not renameable, which is what makes this safe.

### 3.5 The affordance row

An empty container shows one affordance row — the only way to put something
inside it. Its control is a **split control**: a bubble (makes a chat) or a git
mark (makes a worktree), with a small dropdown where both are legal. No
subtitles, no descriptions, no visible dropdown chrome — just the icon; the menu
appears on click. A chat that *could* become a worktree (because its parent is a
valid worktree or repository) carries the same dropdown on its own mark.

**A worktree is never demoted back to a bubble.** Never — not even when clean.

---

## 4. Spaces — one project per sidebar

A project stops being the tree's top level and becomes the **space** the whole
rail is in. One project, one sidebar; you move between them across, in an
x-mandatory snap scroller with `min-width: 100%` panels and no scrollbar.

What that buys is the thing the tree could not: nothing on screen belongs to a
project other than the one you are in. No row carries its project, the deepest
indent drops a level, and Recents is per space for the same reason.

**The space header is the `.row` component with different controls.** No ground
of its own at rest and the same `--accent` under the pointer every other row
takes. Being the space you are in is said by the controls, not by a permanent
fill.

- **At rest** — the project's own mark (the same glyph its row carried in the
  tree) and its name. A control nobody is reaching for is noise in the one place
  that has to say *which project this is*.
- **On hover** — the mark's slot becomes a **chevron**, and an **overflow (`…`)**
  appears.
- **Folded** — the chevron stays, rotated: it is reporting a state, not offering
  one.
- **The panels either side** carry the mark and the name only. Nothing you could
  do to them applies until you get there.

**Clicking the header folds the space: the tree goes, Recents stays.** That is
the point of folding — to see nothing but what is up in this project. They share
one scroller, so the fold hides the rows, not the scroller.

**No rules between rows.** The horizontal separators divided *projects*; a space
holds one project, so its repos are siblings, not sections.

### 4.1 The space marks

The swipe cannot be the only way in: at rest one panel fills the rail, so with
no neighbour visible there is nothing to click and nothing to discover.

**The window-chrome row already has dead space in the middle** — traffic lights
and the sidebar toggle at one end, back / forward / settings at the other. The
space marks go there: **one mark per project, in a row, icon only.** Each is the
project's own mark, the same glyph its space header and its old tree row carried.

- the **current** space's mark is at full strength; the others sit muted
- **click a mark to go to that space** — it is the discoverable counterpart to
  the swipe, and it drives the same scroller, so the mark and the panel are two
  views of one number, exactly as the card's underline and its scroll offset are
- no labels, no counts, no close — a mark and nothing else

This is the third instance of the same pattern in the rail (card head, spaces,
marks): **a set of icon-only marks where the lit one is the state, and the
gesture and the click are the same act.** Build it once.

**Two gestures, two devices.** Switching spaces is a **wheel** gesture; dragging
a chat out to a pane is a **pointer** drag. That is how both can be horizontal
without a modifier. Over a row the pointer drag wins, which is correct.
Consequently `.row` may not set `touch-action: none` on either axis — it is
`pan-x pan-y`, because the rail now scrolls both ways and a row sits under the
pointer everywhere.

---

## 5. Recents — what is up, and what is running

The tree answers *does this exist, and what does it run in*. It has never
answered *is this up right now*, and with panes dividing as far as anyone wants,
that second question has several answers at once.

A **view** is one pane or a set of them. A set draws as one shell with its
members inside it, so an arrangement reads as a single thing you can go back to
rather than as n loose chats.

### 5.1 The band

Header: a hairline rule spanning the width with the word **Recents** at its far
end — the section is titled without spending a row on a heading. Below it, the
entries, at one level. **No indent, no parentage, no chevron, no second line.**

### 5.2 Four states

| state | drawn as |
|---|---|
| **LIVE** | the arrangement on screen. Focused member is `is-active`, exactly as in the tree |
| **WORKING, NO VIEW** | a chat the daemon is running that no pane holds. Spinner, and **no close control** |
| **SET** | two or more chats in one view — a shell around rows |
| **DORMANT** | a closed view. A bare row |

### 5.3 The set

**A shell around rows, not rows welded into a bar.** The wrapper carries the
grouping — its own ground, a slightly larger radius, 2px of padding — and each
member keeps its own pill inside it. Flush segments with a hairline between them
lost the row; this keeps it.

**The shell carries the state, the members carry the pointer.** A set that is up
takes `ROW_ACTIVE` whole — the same `--pane`, border and inset highlight a lone
active row takes, because a set is one row's worth of "this is what you are
looking at". Its members then only answer the pointer, and inside the raised
shell the muted fill is enough to say which one. **At rest the shell and every
member are empty**: nothing is up, so nothing is lit, and the shape alone does
the grouping.

### 5.4 The ×

Every row has one except the working row, and it always means the same thing:
**end this view, never touch the chat.**

- on a **live member** → closes that pane
- on a **remembered** one → forgets the arrangement
- on a **working** row → *absent*. There is nothing left to close, and that
  absence is the signal that it is running with no window on it

**Closing the last pane empties it rather than refusing.** A chatless pane is
the New Tab stage, which this design already draws. A control that is present
and then declines is worse than the state it was protecting.

### 5.5 The sequence this exists for

1. A chat opens into a view → a row appears, with a close control.
2. You close it → **the view dies, the row does not.** Still working, and it
   comes straight back as a working row; idle, and the view it was is remembered
   so the close is undoable.
3. The turn ends → the row leaves on its own.

Whatever ends a view owes it that — **including a drop that takes over a pane**,
which is the same eviction wearing a different gesture.

### 5.6 Population and order

A chat appears **at most once**, in the highest band that claims it: live, then
working, then dormant. The spinner rides the member wherever it lands, so "this
one is running" never depends on which band it fell into.

**Order is the user's, and only a drag changes it.** Raising a working row,
closing a live one, restoring a dormant one — the row stays exactly where it
sits. Recency does not sort this band and neither does state. An entry is keyed
by its **chats**, not by its state, so a view keeps its slot as it changes kind;
an arrangement that gains or loses a pane inherits the place of the one it grew
out of. A row can never leave the band: there is nowhere in the tree it could
mean anything.

### 5.7 Duplication is not the defect

A tree row and a Recents row for the same chat are **not** a duplication to be
removed. They answer different questions — *does this exist* versus *is this up
and running* — and the mirror between them is what makes the pair legible:

- **different verb** — tree row *opens*; Recents row *focuses*. Recents never
  creates.
- **different shape** — no indent, no parentage, no branch subtitle in the band.
- **different information** — the tree says what a chat *is*; Recents says what
  it is *doing*, and pane identity appears only there.
- **mirrored selection** — the focused chat lights in both at once. Two rows that
  highlight together read as a cross-reference; two that highlight independently
  read as two things with the same name.

The empty state teaches it for free: no band until something is open, so the
first time it appears, it appears *because* you opened something.

---

## 6. The file explorer card

**The card takes screen; it never takes layout.** A split rail charges the tree
for the panel at every window height, including the ones with 300px of empty
rail below the last row. A floating card charges nothing until the tree is long
enough to reach it. That distinction is the whole argument for it.

- inset 8px on three sides, `--radius` 12px, `--pane` ground, the pane's own
  elevation, over the tree
- it opens at **one third of the sidebar's height** and is dragged from there
- its **top 6px is the resize handle** — the same hot zone the pane sash uses,
  except here it lands on an edge that is already drawn
- height is kept as a **proportion of the rail**, so it survives a window resize
- the tree keeps a **bottom inset** the height of the card, so the last row can
  always be scrolled clear. Without that inset those rows are unreachable, and
  it is the one standing cost of floating rather than splitting

### 6.1 The head

`ui/tabs.tsx`, `variant="underline"` — a variant that already ships and nothing
in the app uses. No track, no pill, no labels, no divider: **28px against the
segmented bar's 36**, and the eight pixels go back to the tree. The underline is
the only line the head draws, and it draws it under one glyph rather than across
the whole card — a rule between head and body would have said they were two
things, and they are one. It slides; it must not cut.

Icon-only costs certainty about what is selected, so the selected glyph takes
**two marks**: the underline, and full-strength colour against the other's 62%.
The shipped version adds Phosphor fill vs regular on top.

The head holds **two glyphs and nothing else** — Files and Git. There is no
search: the sidebar had one and it earned nothing. The **fold control sits on
the head's own line, at its right** (`ml-auto`), rather than anywhere in the
tree above it. The base tabs list is `w-fit justify-center`; this call site
overrides to `justify-start` with the fold on `ml-auto`.

The head names no scope and carries no scope control. The card acts on the
current worktree, and the current worktree is the row lit in the tree behind it.

### 6.2 Files and Git are one carousel

Not two states of the body. `min-width: 100%` panels, `scroll-snap-type: x
mandatory`, scrollbar hidden — the trackpad moves between them exactly as it
moves between the sidebar's own panels. **The underline and the scroll offset
are two views of the same number**, so clicking a glyph and swiping to it are
the same act.

Both panels are laid out at all times, which is what makes the gesture possible
and also what fixes each panel's own vertical scroll: each keeps its place while
the other is off-screen.

**The gesture is armed by `wheel` or `touchstart` and by nothing else.**
Everything else that moves `scrollLeft` is reflow, not intent: the re-align on a
resize, the smooth scroll when a glyph is clicked, and the browser clamping the
offset to 0 while the card folds and restoring it as it opens. Reading those
offsets back through `Math.round()` lands you on whichever panel was nearest —
the bug the shipped sidebar carousel names, and it reaches here too because the
card can fold.

**Verified:** swipe moves one panel exactly and the underline follows; clicking
a glyph smooth-scrolls; fold→unfold returns to the same panel, placed not
animated; dragging the card's edge in either direction never changes the panel;
a vertical swipe scrolls the panel and leaves the carousel alone.

### 6.3 What the two panels hold

**Files** — the worktree's file tree for the focused chat, rows on the same
`.row` recipe one surface up (the selected row can no longer be `--pane`, so it
takes `--el-idle` with no shadow). Clicking a file opens it **in the editor
view** of the focused pane, never in a pane of its own.

**Git** — top to bottom:

1. a **commit box**: a "Describe the change…" field, then **Commit** and **Pull
   request**;
2. **Review this branch**, carrying the changed-file count, which opens the
   branch review **in the editor view**;
3. `Changed — n files`, then the changed list with `+adds` / `−dels` per row.

The review is not a pane, not a second sidebar, and not a column of its own.
There is one sidebar, so a changed-file list down the left of the review pane is
ruled out — the card already is that list.

**Copy rule for git verbs:** the target is named, the source is not. "Rebase
onto `enhancement/unify-sidebar`", never "Rebase `fix/turn-wedge` onto
`enhancement/unify-sidebar`" — you are standing in the source; it does not need
saying.

### 6.4 Folded

The card keeps its head and drops everything under it. The caret rotates.

---

## 7. The pane

### 7.1 The row across the top

```
[ ▣ split ] [ 💬 chat name ] │ [ tab ] [ tab ] … [ + ]
```

**One row, not two.** Two tab bars said the chat and the files were two systems;
they are one.

- **The split toggle leads the whole row**, before the chat name, outside the
  tab scroller.
- **The chat is not a tab.** No close affordance, no reordering, outside the
  scroller. It is the head of the row.
- **The underline means one thing:** *this is what you are looking at*.
- **`+` is the last child inside the scrolling tab container** — Lucide `Plus`,
  `variant="ghost" size="icon-sm"`, `rounded-sm text-muted-foreground
  hover:bg-sidebar-element-hover`.
- There is **no "Editor" tab**. The second view is reached through its open
  files or through `+`, never through a tab standing for the idea of one.
- A view holding only its chat draws **no bar at all**.

### 7.2 Two views

A pane holds the **chat view** and the **editor view**. The editor view contains
everything that chat has open — files, terminals, branch review — and opens on
the **empty stage**: the Crowbar hero animation, the wordmark, and three ways in
(open a file from the card's Files, run a terminal, review the branch from its
Git). Nothing lands in a pane of its own; everything lands in the editor view.

**Tabs are the default at every size.** Side-by-side is opt-in, and the toggle
is only offered where the pane could hold both.

- **Landscape** → side by side horizontally.
- **Portrait** → stacked vertically, and **the tab strip moves down between the
  chat and the editor**. The chat name stays in the head.
- **Too small, or the split is off** → tabs.

The rule *reading a file must never hide the agent's message* is cheap on
landscape and expensive on portrait. It is therefore not a rule; it is what the
split toggle is for.

There is **no swipe gesture** between the two views.

**Both surfaces stay mounted.** `display: none` dormancy is load-bearing —
content-visibility melted the CPU.

Stacked vs side-by-side is measured on the **pane**, never the window, via
`ResizeObserver`.

### 7.3 The layout is a tree

Panes form a **recursive binary tree**, exactly like the app's `LayoutNode`: a
leaf is a pane, a split holds two children and a proportion. Dropping on a
leaf's edge replaces that leaf with a split holding it and the new one, which is
what makes it divide as far as anybody wants. **There is no cap and no special
case for the second one.**

Exactly one pane is focused; the sidebar and the card scope to it.

**Panes are reorderable.** Dragging the boundary between two moves it; dragging
a row onto a pane places a new one wherever the drop zone says. A pane group is
now a group of *chats*, never of tabs, and the user arranges them however they
like.

### 7.4 Gutters and corners

Percent of the content box, inset by a constant **4px**. **Left and top always
take it**, so the gutter above the first pane is the same as the one beside it
and two neighbours sit 8px apart on either axis. **Right and bottom give it up
at the window**, where the pane is meant to run into the frame.

Border is `2px solid transparent`, becoming `var(--secondary)` when the focus
ring is earned. Radius is `var(--radius-lg)` **unless the adjacent edge is a
window edge**. `pane-border.ts`, verbatim: **top is never a window edge**;
left/right are shielded by the open sidebar on that side; bottom is `atBottom`.

**Why the corners flatten:** a rounded, shadowed corner composited against the
window's own rounded vibrant edge measured 8ms frames becoming **106ms** in
WKWebView — 125fps to 9fps. Keep the rounding "play"; do not round all corners
always.

Sashes take the same insets so the boundary a sash sits on and the gap it lives
in stay aligned as you divide.

---

## 8. Drag and drop

**One arm, one hit test, one drop.** A row in the tree and a row in Recents run
the identical drag: same 5px threshold, same ghost, same targets, same outcomes.

A press becomes a drag only after it **travels 5px** — enough to separate intent
from the shake in a click.

### 8.1 Targets

| target | outcome |
|---|---|
| **middle of a pane** | into this view, you choose where |
| **edge of a pane** | into this view, on that side |
| **middle of a Recents entry** | into that view, **opened** |
| **above / below a Recents entry** | it moves to that slot |

The middle third means *into this one*; anywhere else means *between these two*.
One gesture, two outcomes, told apart by where in the row it ends. No modifier.

Pane zones come from `getPaneDropZoneFromRect`, verbatim: `threshold = 0.25`
with two diagonal tests so corners resolve to the nearer edge.

### 8.2 Rules

- **Every drop adds.** A pane's middle used to swap its chat out, silently
  costing you the view you were in. That gesture is gone.
- **Dropping a chat that is already up** goes *to* it and marks where it is with
  a neutral indicator; it never opens twice.
- **Merging opens, it does not file.** You asked for them side by side, so you
  get them side by side.
- **A target already on screen grows instead of reopening** — reopening would
  file the arrangement you are looking at and then restore a copy of it, leaving
  a ghost behind.
- **Whatever goes up leaves every arrangement that was remembering it**, and the
  arrangement you leave is remembered **minus** whatever you took out of it.
  Pull one chat out of a live three-up and the other two are kept as a set, not
  the three. An arrangement left with nobody in it goes.
- **The entry about to take a drop wears the same ring a pane wears.** Same
  answer to the same question.

### 8.3 Refusals

- A **working** chat may not be dragged. Moving a row re-points the ground under
  it. The rest of the sidebar dims, the dragged row goes red, and a short line
  says why.
- **Cross-repo drag** is legal **only** for a chat with no worktree.
- **Reparenting and deleting both take the whole subtree**, always.
- A bubble always has a parent — the project is the god parent of them all.

### 8.4 Clicking

**Clicking a chat in the tree makes its own view.** It does not take over the
pane you were in — that arrangement is put down whole, into Recents, one click
from coming back. **Nothing you click ever costs you what you were looking at.**

A chat that is already up is gone *to*, never opened twice.

---

## 9. Deleting

It used to live on the content view, and it could: a pane *was* a workspace, so
the verb had a subject. A pane is now a chat in a slot, and a chat is not a
worktree. **The verb goes back to the thing, and the thing is a row.**

- **Every row that owns something carries a trash**: chats, workspaces, folders,
  repos, and the space header for the project.
- **Two never do.** A **protected branch** is the repo's own ground — `develop`
  and `main` are not workspaces you made, so they are not workspaces you can
  delete. An **affordance row** is a gesture, not a thing.
- **It takes the subtree.** A workspace's chats are its own, so deleting the
  workspace deletes them.
- **Where it sits:** leading the trailing cluster, never between the two
  controls you actually reach for (the `+` and the chevron), and it takes the
  deny tint the moment the pointer is on it, so the last frame before the click
  is unambiguous.
- **What it cleans up:** deletion is the only act that removes a *thing* rather
  than a view, so it is the only one that can leave a name behind. It clears the
  layout of any pane holding a deleted chat, plucks every arrangement in Recents
  that remembered one, drops arrangements left empty, and invalidates the
  id→name and id→project caches. If the last pane held something deleted it
  takes the first chat still standing.

**Everything else here is reversible and must stay that way.** This one act is
the exception, and it is the reason none of the others may look like it.

**A working chat is not deletable. REFUSED, not confirmed.** While a turn is in
flight the trash declines and says why; it does not offer to kill the agent. The
whole band exists so a running chat is never invisible, and deleting one out
from under itself contradicts that.

**An idle delete confirms, and the confirm names what goes** — the card knows the
changed-file count, the subtree walk knows the chat count. "Delete workspace" and
"delete a workspace, 6 uncommitted files and 3 chats" are different decisions.

---

## 10. What was rejected, and why

| rejected | why |
|---|---|
| **A second sidebar** for files/git | Hard constraint: two rails do not fit on a vertical monitor |
| **A sidebar switcher** (segmented tabs over four panels) | Costs a row of chrome at every height and makes the tree one view among four; the tree *is* the rail |
| **Hiding files/git behind a button** | Loses the thing the card buys: seeing the tree and the files at once |
| **A switcher between chat and editor** | "Horrible" — the two views are one pane, not two destinations |
| **Two tab bars** in one pane | Said the chat and its files were two systems |
| **An "Editor" tab** | A tab standing for the idea of a view, next to tabs standing for actual files |
| **A swipe between chat and editor** | Removed on request |
| **A two-pane cap** | Divide a pane you just made and it divides again; no cap |
| **Drag-to-trash on the card** | Puts an irreversible drop in a gesture space that is otherwise entirely additive, on a surface whose job is showing files; and it only exists while dragging, so it teaches nothing when you go looking |
| **Recents as a second scroll region** pinned to the floor | The rail is one sidebar and scrolls once |
| **The band listing chats, keyed by bucket** | A row changing kind is not a reason for it to move |
| **A focus exemption on the tree's grey** | Made the tree answer a question Recents already answers, in a different shape |
| **Trash inside an overflow menu only** | Superseded: the trash is on the row |
| **A search glyph in the card head** | Files and Git only; search earned nothing |
| **A divider between the card head and its body** | Would have said the head and the list were two things; they are one |
| **A two-line row for the active workspace** | Superseded once Recents existed — see §3.3 |

Three rules were written for the **sidebar switcher** before it was removed
entirely, and are recorded only so nobody re-derives them: unselected items
showed icons alone while the active one took icon + label; it had to be far
smaller in height than first drawn; and it had to be the app's real tab
component rather than a lookalike. **The switcher is gone. None of this
applies.**

---

## 11. Numbers, lifted not invented

| value | source |
|---|---|
| `SPLIT_SIDE_BY_SIDE_MIN_PX = 780` | `use-chat-presentation.ts` |
| `SPLIT_MIN_HALF_PX = 340` | `use-chat-presentation.ts` |
| `SPLIT_MIN_STACKED_PX = 160` | `use-chat-presentation.ts` |
| `SPLIT_DEFAULT_SIZES = [45, 55]` | `use-chat-presentation.ts` |
| drop-zone `threshold = 0.25` | `pane-drop-zones.ts` |
| pane border `2px solid transparent` | `pane-border.ts` |
| top is never a window edge | `pane-border.ts` |
| pane gutter `4px` / neighbours `8px` | this spec, §7.4 |
| card head `28px` (vs segmented `36px`) | this spec, §6.1 |
| card resize hot zone `6px` | matches `pane-sash.tsx` |
| row `h-9 / px-1.5 / mx-1.5 / my-0.5 / gap-1.5 / 13px` | `ROW_BASE` |
| project header row `44px`, traffic-light reserve `72px` | `sidebar-project-header.tsx` |
| Recents separator `22px` | this spec, §5.1 |
| drag threshold `5px` | this spec, §8 |
| sole New Tab has no close (`buffer.isUncloseable`) | `tab-bar.tsx` |
| carousel armed only by `wheel` / `touchstart` | `sidebar-carousel.tsx` |
| `clientWidth === 0` guard while collapsing | `sidebar-carousel.tsx` |
| rounded+shadowed corner vs window edge: 8ms → 106ms | measured, WKWebView |

`agentChats.working` is a `Record<chatId, boolean>` on the workspace store, fed
by the daemon's turn feed and **tracked independently of `buffers`**. That is
what makes law 9 implementable: a chat can be working with no view, and the tree
row already subscribes to it per-row (`agent-chats-panel.tsx` deliberately does
not subscribe to the whole map — immer replaces its reference on every turn
frame).

---

## 12. Decisions closed

The six questions this design left open are answered. None of them is open any
more; they are recorded here with their answers so the next session does not
reopen them.

1. **Deleting while an agent is working — REFUSE.** Not confirm-and-kill. The
   band exists so a running chat is never invisible; deleting one out from under
   itself contradicts the one law it was built for. The confirm for an idle
   delete still has to **name what goes** — the card knows the changed-file
   count, the subtree walk knows the chat count.
2. **Recents below the fold — keep it.** One scroller, as designed. The tree and
   the band move together and the band is reached by scrolling. This was
   considered and chosen; it is not a defect to be fixed later.
3. **Switching spaces — add the space marks.** See §4.1.
4. **Touch — out of scope.** Crowbar is desktop only. The horizontal-wheel /
   horizontal-pointer split is sufficient and needs no touch fallback.
5. **Close controls in a set — moot.** The real interface reuses Crowbar's
   existing row component rather than anything drawn in the prototype, and that
   component already has a working answer for trailing controls at width. This
   is a rewiring, not a new component.
6. **Migration — none.** Build as if shipping for the first time into an empty
   world. No backfill, no compatibility path, no dual-read.

---

## 13. Handoff

**This document is the frontend half.** A companion session writes the backend
implementation spec from
[`2026-08-23-unified-sidebar-design.md`](./2026-08-23-unified-sidebar-design.md)
and this one; the two are built together.

What the frontend work is, in one line each — all of it rewiring existing
components:

| surface | what changes |
|---|---|
| rail | gains the spaces scroller; loses the tab bar, the context pill and the New Project row |
| window chrome | gains the space marks in its dead middle (§4.1) |
| space header | the project row, promoted; row component, different buttons |
| tree | one project's rows; no rules between them; no second line; open rows grey; trash on every row that owns something |
| Recents | new band at the tail of the same scroller; rows are the same row component |
| card | stays floating and last; head becomes the underline tabs variant; Files/Git become one x-snap carousel |
| pane | holds one chat; one row across the top; two views; recursive binary layout tree |
| drag | one arm, one hit test, one drop, shared by tree rows and Recents rows |

**Nothing in the prototype is a component to port.** It is a description of
behaviour that was driven and measured; the implementation reuses what Crowbar
already ships.

---

## 14. Provenance

The interactive prototype lives in the design canvas published for this session.
Every behaviour in §5, §7 and §8 was driven and measured there — pane splitting
to four levels, the carousel, the drag parity matrix, the delete cascade, and
the close-to-empty-stage fallback — before it was written here.
