# New Tab: the empty stage as a real tab

**Status:** IMPLEMENTED 2026-07-22 on `enhancement/restyling`, not merged, not pushed.
Awaiting live verification in the Tauri app (see the plan's manual checklist).
**Surface:** `empty-editor-state.tsx` deleted → `features/panes/components/new-tab-view.tsx`

> **Two deviations from this spec, agreed during implementation:**
> 1. The cluster is **centred** in the pane, not anchored to its floor. The
>    floor-anchored composition was the original premise; the centred one was
>    preferred on sight. Its contents stay left-aligned regardless — centring the
>    action rows and their chord badges would ragged-edge a list.
> 2. `--logo-ink` lives in two theme blocks, not four — see the corrected note
>    under "Details" below.
**Related:** [artifact mockup](https://claude.ai/code/artifact/974f790d-3acf-4a9e-ab2e-ef04b3f87086)

## Why

A pane with nothing open currently renders a 192px box holding a single ghost
button — "New Terminal" — and nothing else (`empty-editor-state.tsx`). It says
nothing about where you are, offers no way to start a file or a chat, and
surfaces none of the worktree's agent history. The same component is the whole
of Project Home, which is a real (non-git) workspace under the hood and lands on
the identical surface.

Two changes land together:

1. **The surface is redesigned** — the "Nameplate" layout below.
2. **The surface becomes a tab.** It stops being a `!activeBuffer` fallback and
   becomes a real buffer you can open with a chord, switch away from, and see in
   the tab strip. A pane is never tab-less.

## What already exists

The `newTab` buffer type is **already defined and already dead**. This design
turns it on rather than inventing it:

| Location | Today |
| --- | --- |
| `pane-content.ts:73` | `NewTabContent` interface exists |
| `pane-content.ts:228` | `{ type: 'newTab' }` is a valid `OpenContentSpec` |
| `buffer-slice.ts:229` | `openContent` can mint one, named "New Tab" |
| `tab-bar.tsx:63` | **filters `newTab` out of the tab strip** |
| `pane-container.tsx:131` | **skips rendering `newTab`** |
| anywhere | nothing ever calls `openContent({ type: 'newTab' })` |

So a `newTab` buffer would currently be a ghost: present in state, active, but
with no tab and no content. Two suppressions to reverse, one component to write.

Also already present and load-bearing for this design:

- `isUncloseable` on `PaneContentBase` — `tab-bar-item.tsx:185` already hides a
  tab's × when set.
- `isVirtual` on the editor `OpenContentSpec` — untitled buffers are supported,
  which is how New File survives a locked worktree (see below).
- `keybindings-settings.tsx` — a live key-capture rebind UI, gated on
  `command.liveEditable`. Any command we add with that flag is user-rebindable
  with no extra work.

## The surface: "Nameplate"

One cluster anchored to the **floor** of the pane. Nothing is centred, and there
is no background graphic. Reading order is top-down: **isologo → worktree → what
you can do in it.**

```
┌──────────────────────────────────────────────────────┐
│  ‹  ›                                              +  │  tab strip
├──────────────────────────────────────────────────────┤
│                                                       │
│                                                       │
│   ▓▓ Crowbar                                          │  isologo, brand green
│   enhancement/restyling                               │  branch
│                                                       │
│   □ New File      ⌘N     ⣿ Rename workspace…   now    │
│   ▣ New Terminal  ⌃`     ✳ Fix sidebar row…    12m    │
│   ◌ New Chat      ⌘⇧N    ◎ Terminal DOM…       1h     │
│                          21 more in this worktree ⌘2  │
└──────────────────────────────────────────────────────┘
```

### Rules that hold it together

Every rule below was derived by measuring the mockup at five pane shapes, not by
eye. The failure each one prevents is stated, because a rule without its failure
gets "simplified" away later.

1. **Size the isologo against the pane's shorter side**, never a fixed px:
   `clamp(108px, 15cqw, 152px)`. A fixed width towers in a portrait pane and
   shrinks to a smudge in a split.
2. **Query the pane, not the viewport.** All breakpoints are `@container` on the
   pane body (`container-type: size`). A pane's size has nothing to do with the
   window, so a media query is wrong in every split.
3. **Both columns bottom-aligned, rows the same height.** `.act` is a `<button>`
   and `.chat` a `<div>`; their default line boxes differ by ~1px and the columns
   drift out of step. Pin `min-height: 30px; line-height: 18px` on both.
4. **Columns go side by side at `min-width: 700px`**, stacked below it.
5. **History is capped at 3 + a hand-off row.** Uncapped, 12 chats push the
   cluster 92px past the top of a 400px pane and clip the isologo; 24 push it
   476px and take the action buttons off-screen — in a pane that is
   `overflow: hidden`, so they are unreachable, not merely scrolled away. The cap
   equals the number of actions, so both columns stay the same height whatever
   the worktree has accumulated.
6. **History hides only when stacked *and* short**
   (`max-width: 699px and max-height: 380px`), plus a floor at
   `max-height: 235px` for the side-by-side case. Gating on height alone — the
   obvious way — hides history in a wide-and-short pane where it sits *beside*
   the actions and costs only 32px.

### Details

- **Isologo** is `CrowbarWordmark` (`components/ui/crowbar-wordmark.tsx`), filled
  with a new `--logo-ink` token, defined per-theme rather than as a descendant
  override on the component.

  > **Corrected 2026-07-22 during implementation.** This section originally called
  > for four theme blocks (`:root`, `@media (prefers-color-scheme: dark)`,
  > `[data-theme='dark']`, `[data-theme='light']`). That is the mechanism the
  > standalone design mockup used, and it is **not** how this app themes. Here
  > `theme.css` has exactly two colour-token blocks — `:root` and `.dark` — and
  > `data-theme` carries the theme *id* (`crowbar` / `zen`), not a light/dark
  > literal; `data-theme-type` carries that, and `settings-effects.ts` toggles the
  > `.dark` class (following the OS in "system" mode). So the token goes in
  > `:root` and `.dark`, full stop. Light ground gets `--primary`; dark ground gets
  > the lighter brand green.
- **Running chat glyph** is `FlickerSpinner`
  (`components/ui/spinners/tiny-spinner.svg`) via the existing `AgentChatGlyph` —
  not a coloured dot. Keep it on `--muted-foreground` like its neighbours;
  `agent-chat-glyph.tsx` is explicit that colour belongs to the host row.
- **Chat rows** reuse `AgentChatGlyph` so a chat can never read as busy here and
  idle in the sidebar.
- **The hand-off row** ("N more in this worktree", `⌘2`) focuses the Chats
  sidebar tab. The empty pane is not a chat browser.

## Tab lifecycle

| Trigger | Behaviour |
| --- | --- |
| `⌘T` | Open a New Tab in the active pane. If an **untouched** New Tab already exists there, focus it instead of minting a second identical blank. |
| Act inside a New Tab | The result **replaces that tab in place** — New Terminal turns the New Tab into the terminal tab, it does not open a second tab. |
| Another tab opens in the same pane | An untouched New Tab is **consumed** (replaced), like a VS Code preview tab. A tab opening in a *different* pane leaves it alone. |
| Close the last tab in a pane | A New Tab is spawned. **A pane is never tab-less.** |
| Close a New Tab that is the only tab **in the last pane** | It is `isUncloseable` — the × is hidden and `⌘W` no-ops. Otherwise closing it would spawn another New Tab forever and the pane could never be emptied. |
| Close a New Tab that is the only tab **in a split** | `⌘W` closes the **split pane** — that is how a split is dismissed. The × stays hidden either way, because closing the tab itself is never the action. |
| Workspace opens with no restored buffers | Opens on a New Tab. |
| **Project Home** | Behaves exactly like any other workspace — it gets a New Tab too. No special-casing. |

"Untouched" means the New Tab has had no action invoked from it. Once it has been
replaced it is no longer a New Tab at all, so the flag needs no separate storage.

The `!activeBuffer` branch in `pane-container.tsx:592` is **kept, not deleted**,
and repointed at the same `NewTabView`. Under these rules it should be
unreachable — but if a bug ever does leave a pane tab-less, the fallback renders
a usable surface instead of a blank rectangle with no way out. Same component
either way, so there is no second UI to keep in sync.

### Persistence

A `newTab` buffer is **not** persisted across sessions — it carries no state, and
restoring one would fight the "workspace opens with a New Tab" rule. Exclude it
in `lib/persistence/hydrate.ts` at snapshot time, and let the open-with-New-Tab
rule reconstruct it.

## Keymap changes

`⌘T` moves off New Terminal. All four commands are `liveEditable: true`, so they
are rebindable in Settings → Keybindings with no extra work — the existing
key-capture UI in `keybindings-settings.tsx` already handles any command carrying
that flag.

| Command id | Label | Default | Note |
| --- | --- | --- | --- |
| `tabs.new` | New tab | `mod+t` | **new** — was New Terminal |
| `tabs.newTerminal` | New terminal tab | `mod+j` | **changed** from `mod+t` |
| `tabs.newFile` | New file | `mod+n` | **new** — currently unbound |
| `agent.newChat` | New chat | `mod+shift+n` | **new** — currently unbound |

No settings chord is added. Settings stays click-only via the gear in
`sidebar-project-header.tsx:70`.

`mod+j` collides with nothing in the registry today (`mod+\`, `mod+shift+\`,
`mod+alt+arrows`, `mod+t`, `mod+shift+t`, `mod+w`, `mod+s`, `mod+shift+s`,
`mod+k`, `mod+1`–`mod+4`).

### Known issue: `mod+j` double-fires in a focused terminal off macOS

`terminal.tsx:548` deliberately passes **Ctrl combos without Cmd** through to
xterm ("Ctrl+U, Ctrl+C, …"), and `use-pane-keyboard.ts` is a bare `window`
`keydown` listener with **no terminal-focus guard**. So:

- **macOS** — `mod+j` is `⌘J`. The terminal handler returns `!event.metaKey` →
  `false`, xterm suppresses it, only the app acts. Correct.
- **Linux / Windows** — `mod+j` is `Ctrl+J`, which is `^J` (0x0A, line feed).
  xterm sends it to the shell **and** the window listener opens a new terminal.
  Both happen.

This is **pre-existing, not introduced here** — `mod+t` has the same shape today
(`Ctrl+T` is readline's transpose-chars). `mod+j` merely makes it more visible,
because a stray line feed is more disruptive than a stray transpose.

**DEFERRED — macOS is the only target for now (decided 2026-07-22).** `⌘J` does
not reach xterm, so the collision cannot fire on the shipping platform and no fix
lands with this work.

The fix, for whoever picks up non-macOS support: in
`attachCustomKeyEventHandler`, before the "Ctrl combos → xterm" line, return
`false` for any event matching a registry chord. That routes app chords from one
place and fixes `mod+t` at the same time, rather than bolting a focus guard onto
every hook. **Anything binding a Ctrl-letter chord that readline also uses is
broken off macOS until that lands** — this is a platform-support prerequisite,
not a bug introduced by this change.

### The badges must read from the keymap

The chord badges rendered in the surface **must** resolve through
`useEffectiveKeymap()` / the registry, never hardcoded strings. Otherwise
rebinding New Terminal in Settings leaves the surface confidently displaying a
chord that no longer works. This is the main reason the four commands above are
registry entries rather than inline handlers.

## New File on a locked worktree

`use-file-explorer-context-menu.tsx:235` hides New File entirely on locked
worktrees — protected branches and Project Home — because it needs a target
directory and write access.

**Resolution:** New File here opens an **untitled virtual buffer**
(`openContent({ type: 'editor', isVirtual: true, … })`), which needs neither. Only
*saving* needs a path, and that is already a save-as flow. So the action stays
present and enabled everywhere, and the three-action column never has to degrade
to two.

This supersedes the earlier plan to hide the row on locked worktrees.

## Files touched

**New**
- `web/src/features/panes/components/new-tab-view.tsx` — the surface
- `web/src/__tests__/features/panes/components/new-tab-view.test.tsx`
- `web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`

**Modified**
- `web/src/features/panes/components/empty-editor-state.tsx` — deleted
- `web/src/features/panes/components/pane-container.tsx` — render `newTab` (drop the `:131` skip); repoint the `!activeBuffer` branch (`:592`) at `NewTabView`
- `web/src/features/tabs/components/tab-bar.tsx` — stop filtering `newTab` (`:63`)
- `web/src/features/tabs/components/tab-bar-item.tsx` — New Tab label + mark icon
- `web/src/features/workspace/stores/slices/buffer-slice.ts` — replace-in-place, consume-untouched, spawn-on-last-close, `isUncloseable` when sole tab
- `web/src/features/panes/hooks/use-pane-keyboard.ts` — `tabs.new`, retarget `tabs.newTerminal`, `tabs.newFile`
- `web/src/features/keymaps/registry.ts` — four command entries
- `web/src/lib/persistence/hydrate.ts` — exclude `newTab` from snapshots
- `web/src/styles/theme.css` — `--logo-ink` in all four theme blocks

## Testing

Per `CLAUDE.md`, tests live in `web/src/__tests__/` mirroring `web/src/`, with
`@/` imports. Per house rule, **no sleeps, `Eventually`, or poll timeouts** —
block on real signals.

Store behaviour (`buffer-slice`):
- `⌘T` twice in one pane yields **one** New Tab, focused
- acting inside a New Tab replaces it — buffer count unchanged, id reused or
  swapped in the same pane slot
- opening a file in the same pane consumes an untouched New Tab; opening in a
  different pane does not
- closing the last tab spawns a New Tab
- a sole New Tab is `isUncloseable`
- a `newTab` buffer is absent from the persistence snapshot

Surface (`new-tab-view`):
- renders three actions and at most three chat rows + hand-off row at 24 chats
- chord badges reflect a **rebound** keymap, not the defaults (the regression the
  registry indirection exists to prevent)
- running chat renders `AgentChatGlyph` in its working state

Not unit-testable, must be checked live in the Tauri app:
- the isologo's green against **composited vibrancy**, not `--pane-background` —
  native vibrancy lightens whatever sits on it, so a colour picked against the
  token is optimistic
- the surface at a genuinely portrait display, since the layout rules above were
  measured in a browser rig rather than in the real window
