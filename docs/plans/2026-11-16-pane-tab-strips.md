# Pane tab strips: groups of stacked panes inside a tab

Status: DECIDED, in implementation on `migration/rust-shell`. Sources: `shell-spec/tabs-pane-strip.md`, `shell-spec/pane-layout-intent.md` (F2, F4, F5, F6, F8, F9), `shell-spec/pane-layout-chrome.md` section 9, and the owner decisions in `2026-11-14-gpui-shell-migration-plan.md`. No open questions: where the spec is silent the default is chosen and recorded here.

## 1. Problem

A model tab is a split tree whose leaves are single panes. Every new terminal, browser, file, review or New Tab page therefore opens as a split: `ctrl+j` three times gives three narrow columns. The spec says the opposite: chats are split-only, and terminals, files, browsers, reviews and New Tab pages are tabs in a pane's strip (editor tabs). The console is a dock outside the layout.

## 2. Vocabulary

- **Group**: a leaf of the split tree. It holds an ordered list of panes (its strip), exactly one of them active (the one shown). A group of one pane draws no strip.
- **Strip**: the 42 px row above a group with two or more panes, the tabs being the group's panes in order.
- **Stackable pane**: a pane that may share a group. Decided by the app from the pane type, never by the model.
- **Dock**: the console, outside the layout tree and outside layout persistence.

## 3. Which panes stack

| Pane type | Placement | Why |
|---|---|---|
| chat (`core.chat`) | split only: always alone in its group | one chat per pane, the unit of the layout (F2) |
| terminal | stacks | spec tabs-pane-strip s.4 icon list |
| editor file / untitled / host file | stacks | same |
| browser | stacks | same |
| branch review | stacks | same |
| New Tab page | stacks | same |
| console | dock, never in the tree | pane-layout-chrome s.9 |

The side-by-side chat plus editor halves of the old app (`editorOpen`, collapsed "tabs" mode with the chat as first tab) are replaced by explicit layout: a chat is a leaf, its files are a group beside it. This removes the two meanings of `editorOpen` (F8), the 780 px presentation flip (F3) and the hidden `chatSelected` state. Files keep their `threadId` param, so they still belong to a chat's workspace; they are simply not part of the chat's leaf.

The model enforces nothing about types. The app asks one function, `stack_policy::kind_of(type_id)`, which returns `Split`, `Stack` or `Dock`, and chooses the command. The model guarantees the structure whatever is sent.

## 4. Where a new pane opens

Every open starts by looking for the same thing already open (type plus params): if found it is moved or focused, never duplicated (existing rule, kept).

For a `Stack` pane the target group is, in order:
1. the focused group, when its active pane is stackable (all members of a group are stackable by construction);
2. else the first stackable group of the tab in layout order;
3. else a new group split to the right of the focused pane (the tab's focused group; a tab with no panes just gets the leaf).

The new pane is appended at the end of the strip, becomes active in its group and takes focus. For a `Split` pane (chat) the rule is unchanged: right of the focused group unless a drop placement says otherwise. Opening into an empty layout creates a tab as today.

Keyboard: `ctrl+j` terminal, `ctrl+t` New Tab page, `ctrl+shift+n` new file, "New Browser", Review, file opens from the sidebar, all follow this rule. New Tab page replacement semantics (choosing an action replaces the New Tab page in its slot) are done by the app: open the new pane at the New Tab page's index, then close the New Tab page.

## 5. Focus rules (derived, never resynced)

1. A tab has exactly one focused pane (or none when empty) and it is always the active pane of its group.
2. Focusing a hidden member makes it the active pane of its group.
3. A newly opened pane, and a pane moved into another group (`MoveToGroup`), is active and focused. A pane dropped on an edge (`MovePane`) keeps the previous focus, as split moves did before.
4. Closing a member: if it was active, the member now at the same index becomes active, else the last one (spec 5.1); if it was the focused pane the new active pane takes focus; if its group had other focus nothing else changes.
5. Closing the last pane of a group removes the group; the sibling that absorbs the space is where focus goes (as for single panes today), and the closed pane's tab closes when it was the tab's last pane, showing the adjacent tab.
6. Pane navigation (`mod+alt+arrow`) moves between groups by geometry (app `focus_neighbour`), focusing the active pane of the target group.

## 6. Close semantics

- Closing a strip tab (`x`, middle click, `mod+w`) closes that pane only. `mod+w` closes the active strip tab; closing a split group is the pane handle's job (owner decision).
- Closing a chat never touches files, terminals or browsers: they live in their own groups, so the owner rule "editor tabs move to a pane of the same workspace, or the user is asked" holds by construction (nothing is moved, nothing is lost, nothing attaches to an unrelated chat).
- Closing a whole group (handle `x`) closes all its panes in one undo step. The app asks first when any member has unsaved state, as the Unsaved Changes dialog does today; the model is not involved.
- Last pane of a tab closed: the tab closes (existing law).
- Undo restores the strip, its order and which tab was active.

## 7. Drag and drop

Drag subjects are the existing ones: a chat from the sidebar, an open pane (a chat handle or a strip tab), a whole tab. Drop zones on a group are the existing five: four edge bands and the centre. The strip row is its own zone for stacking.

| Dragged | Dropped on | Result |
|---|---|---|
| chat (open or from the sidebar) | edge of any group | split on that side; an open chat is moved, never duplicated |
| chat | centre of a group | split to the right (owner decision, preview shows the right half) |
| chat | strip row | same as centre: a chat never joins a strip |
| strip tab | strip row of the same group | reorder to the insertion index |
| strip tab | strip row of another group (same or other tab) | joins that group at the insertion index, active and focused |
| strip tab | centre of a stackable group | joins at the end |
| strip tab | centre of a chat group | split to the right of the chat, new group of one |
| strip tab | edge of any group | split on that side as a group of one; dropping the only tab of a group on its own edges is a no-op (no preview) |
| strip tab | edge of own group, group has other tabs | the tab leaves the strip and splits that side |
| any | own position (same group, same index) | no-op, no undo step |
| strip tab | a tab in the top bar (other model tab) | joins that tab: stackable group if one exists, else a new group right of its focused pane |

Previews: only drops that will do something draw a preview, and the preview is exactly the result (right half for centre-on-chat, a strip insertion line for strip drops). Cross-project drops are refused by the sidebar owner, not the model. Dragging out of a strip needs a 6 px activation distance (spec 5.4) and the dragged tab becomes active.

## 8. Sizes

Unchanged: one minimum (`MIN_PANE_EXTENT`, 160 px), a 6 px sash, ratios stored as share of the space after the sash, double-click on a sash gives equal shares (`ResetSplit`). A group is one leaf and has no size state of its own, so stacking never changes a ratio. Removing or adding a group keeps the ratios of all untouched splits.

## 9. Console dock

The console is not a pane of the tree and not in the saved layout. Its state is `{ open, dock: top|right|bottom|left, mode: overlay|push, size }` with defaults closed, top, overlay, 420 px (min 160, max 80 percent of the window axis), kept in the window memory file next to window geometry. `ctrl+backtick` toggles it, Escape closes it, overlay mode closes on an outside press, push mode shrinks the layout area. Phase 2 of this plan, owned with the shell view (it wraps the whole shell, so it is not a layout change); until it lands the toggle opens the console as a stackable pane so the log is reachable. This is a recorded deviation, not the intended end state.

Decision: the dock is not part of the tab-strip change and is deferred to its own plan, `2026-11-16-console-dock.md`, a tracked gate item. Three reasons it cannot be a small hook: the console is a native pane, so in overlay mode it floats over native webview holes that sit above GPUI and need the frozen layer to be covered; push mode changes the area the shell hands to `visible_panes`, which `shell_view.rs` owns; and its state (dock, mode, size) needs a field in the window memory store that other agents are editing. Until then the console toggle opens a stackable pane in the focused strip (`stack_policy`), so the log stays reachable.

Tests first for what remains: a dock-state test (defaults, cycle top-right-bottom-left, size clamp 160 to 80 percent, persistence round trip outside the layout file) must fail before the dock code is written.

## 10. Model changes

`SplitNode::Leaf` holds a `Group` instead of one pane id.

```
Group { panes: Vec<PaneId>, active: PaneId }      // non-empty, active is a member
Tab::groups(bounds) -> Vec<GroupSlot>             // GroupSlot { rect, panes, active }
Tab::layout(bounds) -> Vec<PaneSlot>              // unchanged shape: the active pane of each group
Tab::group_of(&PaneId) -> Option<&Group>
Tab::group_count()
```

`pane_ids()` lists every pane (hidden ones included) in layout order, strip order inside a group. `len()` counts every pane. `MAX_PANES_PER_TAB` stays the cap.

New commands (all one undo step, atomic, structural):

- `OpenPaneInGroup { instance, onto, index }` (the tab is the one holding `onto`): opens a pane as a member of the group holding `onto`, at `index` (end when `None`). Same-identity panes are moved instead (as `OpenPane`).
- `MoveToGroup { pane, onto, index }`: moves an open pane, from any group of any tab, into the group holding `onto`; within one group it reorders. The destination tab becomes active; an emptied source tab closes.

Existing commands become group aware without changing shape:

- `FocusPane`: activates the pane in its group.
- `ClosePane`: removes one member (rules in sections 5 and 6).
- `MovePane` / `MovePaneToTab`: take the pane out of its group (the group stays when it has others) and split it beside the target as a group of one. Moving the only pane of a group onto its own group stays `SamePane`.
- `DetachPane`: refused only when the tab has a single pane.
- `SwapPanes`: exchanges the two panes' places, also across groups.
- `OpenPane`, `AddTab`, `CloseTab`, `MoveTab`, `ResizeSplit`, `ResetSplit`, `SetPaneState`: unchanged.

New errors: `IndexOutOfRange` is reused for strip indices; no other variant is added.

Invariants checked by `Tab::validate` on every load: every group non-empty with no duplicate, `active` among the members, the union of groups equals the pane set, the focused pane is the active pane of its group.

## 11. Persistence and migration

`SCHEMA_VERSION` goes 1 to 2. The leaf shape changes from `{"node":"leaf","pane":"a"}` to `{"node":"leaf","panes":["a","b"],"active":"a"}`. Migration 1 to 2 walks every tab tree and rewrites each leaf to a group of one; it never merges panes, so an old layout keeps its arrangement exactly and only new opens stack. Migrations are pure JSON functions in `schema.rs`, applied in order from the stored version; a newer version is refused; a file that fails an invariant after migration is quarantined by the app (existing `load_or_quarantine`) and the shell starts fresh. The file is written atomically through the existing `LayoutWriter` (temp file, fsync, rename); nothing about writing changes.

## 12. Rendering

- `pane_strip` crate: the strip as one element. Row 42 px, padding 8/4, gap 6, tabs 32 px high, up to 200 px wide, radius 6, label 13 px medium, icon box 14 px, close button 20 px at the right (0.6 opacity when active or hovered), active tab filled with the sidebar hover colour, drag source 40 percent opacity, insertion line while dragging. Colours come from the `ui` palette; no new tokens.
- `layout_view`: a group with two or more panes reserves the strip height at the top of its content, like a chrome strip; the hole of the active pane starts below it. Hidden members get no hole, so the pane host parks them (state kept). The pane handle shows for any tab with two or more groups.
- Accessibility: the strip has tablist/tab roles in the accessible tree and arrow key navigation when focused (parity checklist item, tracked in the parity pass).

## 13. App wiring

`pane_commands.rs` (ctrl+j, browser, new file, review, files) and `pane_opener.rs` build `OpenPaneInGroup` through one function, `stack_target(layout, kind)`, implementing section 4. Strip clicks send `FocusPane`, strip close sends `ClosePane`, strip drags send `MoveToGroup` or the split commands of section 7. The app owns `kind_of`; the model never sees a type.

## 14. Test matrix (model)

Table-driven, per command: stacked instances per slot (1, 2, many), open at end / at index / out of range, activate by focus, close first / middle / last / only member / focused / non-focused / active-not-focused, reorder forward / backward / same place, move into a group of the same tab / another tab (source emptied or not), drop-out to each edge with the group staying or vanishing, swap across groups, detach from a group, undo and redo of each, identity dedupe across groups, tab cap, persistence round trip of stacked groups, v1 layouts migrating to groups of one, a v2 file with each broken invariant refused, newer version refused. Every command is also checked against the validity invariants after it runs (property loop over a sequence of commands).

## 15. Order of work

1. Plan (this file). 2. Model with tests first. 3. `pane_strip` and `layout_view` rendering and interactions. 4. App wiring. 5. Live check under Xvfb. 6. Console dock.
