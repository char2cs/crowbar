# Shell spec: pane layout behaviour, intent versus observed bugs

Companion to `pane-layout.md` and `pane-layout-chrome.md` (visuals are 1:1 there). Owner rule: the Rust side implements INTENT, never today's quirks. Each flow has `Intent` (what a user expects) and `Observed bugs` (what current code does, `file:symbol (~line)`, reason). Paths relative to `app/panes/src/features/panes` unless stated. Intent items are my reading of the code comments and normal desktop expectations; items marked OWNER need a decision.

## F1. Open a chat (click in sidebar, new chat)

Intent: a click shows that chat's tab; if the chat is already open anywhere, that tab and pane get focus; a new chat opens as its own tab beside the current tab and becomes active; another project's chat never steals the screen.

Observed bugs:
- Opening while the stage is showing fills the stage pane and promotes it (`stores/slices/pane-actions/view-actions.ts:openChat` ~38-50): the new tab's id is the stage pane's id and a fresh stage pane is minted, so ids are reused across roles. Reason: the stage doubles as the first tab. Rust: model "no tab open" as an empty state, not as a hidden tab.
- A foreign-project open only records `activeViewByProject` and does nothing visible (`view-actions.ts:openChat` foreign branch), so the click appears dead. Intent: switch project or show feedback.

## F2. Drop a chat on a pane (the only way to make splits)

Intent: left, right, top, bottom edge splits that side; centre of an empty pane fills it; centre of an occupied pane adds the chat beside it (OWNER: side); an already-open chat is moved, not duplicated; dropping a chat on its own pane is a no-op; cross-project is refused with a message; the drop preview matches what will happen.

Observed bugs:
- Centre on an occupied pane silently becomes "split right" (`pane-actions/chat-drop-actions.ts` ~28) while the preview overlay for centre is the whole pane (`components/split-drop-overlay.tsx` zoneStyles.center). Preview lies. Rust: preview the right half, or make centre mean something else.
- Drop on self: existing chat in the same pane and zone is an edge: `existing !== paneId` guard skips the move but still `focusPane` (~46-49); the overlay still drew a split target. Intent: no preview and no-op for the own pane.
- File-explorer drags set the same hover store and draw the split overlay (`stores/drag-store.ts:setDropHoverAt`) but the drop ignores the zone and just opens the file in that pane (`features/file-explorer/.../use-file-explorer-drag-drop.ts` ~217). Preview lies.
- Editor-tab drags draw split zones too (`hooks/use-pane-drop-handlers.ts:handleSplitDrop` ~62-81): non-centre only activates the tab, centre re-adds it; no split ever happens. Preview lies. Rust: tab drag shows no pane overlay (reorder only).
- Cross-project check allows the drop when either project is unresolved (`components/sidebar/lib/pane-drop-actions.ts:openChatIntoPane` ~146, comment "a sidebar one frame behind"): the guard is time-dependent.
- A move across views via `movePane` does not re-clamp or reuse the target's ratios: the moved pane takes a fresh `[50,50]` split (`utils/pane-layout.ts:insertLeaf`), so the target pane's existing space is halved even in a deep nest.

## F3. Split ratios and resizing

Intent: dragging a divider follows the pointer exactly, honours a single consistent minimum, and the saved ratio equals the released position; double-click resets to equal; ratios survive restart; chat/editor ratio is remembered per pane.

Observed bugs:
- Two minimums from one constant: `MIN_PANE_SIZE = 10` is a 10 px drag floor (`components/pane-sash.tsx` default `minPx`) and a 10 percent floor on commit (`utils/pane-layout.ts:normalizeSizes` ~24-32). A pane dragged to 10 px jumps to 10 percent on release (`constants/pane.ts`). Rust: one minimum in px, applied while dragging and on commit; 10 px is unusable, use the chat's 340 px / 160 px or an owner value (OWNER).
- `distributePaneSplit` exists, nothing calls it: no reset gesture (`pane-actions/layout-actions.ts` ~60-70).
- Panes' basis percentages sum to 100 while the 6 px sash is extra; layout depends on `shrink` to absorb it (`components/pane-node-renderer.tsx` ~80-95), so rendered ratios differ from stored ones by up to sash/size. Rust: lay out as (size - sash) split by ratio.
- Chat/editor ratio lives in component state (`components/pane-container.tsx:splitSizes` ~79): lost when the pane remounts, when the view is parked and unmounted, and on restart. Intent: persist with the pane.
- The sash is not keyboard accessible and has no ARIA value (`pane-sash.tsx` render).
- Switching presentation (side-by-side, stacked, tabs) by crossing 780 px has no hysteresis (`agent/hooks/use-chat-presentation.ts:derivePaneViewPresentation` ~139-145): resizing the window around 780 px flips layouts back and forth. Also before the first measure it guesses side-by-side (width and height 0) then may correct. Intent: add hysteresis or measure synchronously.

## F4. Close a pane / chat (handle X, closePane)

Intent: closing a chat removes only that chat; the remaining panes expand into the freed space; focus goes to the neighbour that took the space (or the most recent pane); editor tabs of the closed chat either go with it or stay visible, never silently attach to an unrelated chat; closing the only chat of a tab closes the tab and shows the adjacent tab; nothing unsaved is lost without a prompt.

Observed bugs:
- Focus after close goes to the FIRST LEAF of the remaining layout, not the sibling that absorbed the space or the MRU pane (`lib/view-ops.ts:removePane` ~`const survivor = getFirstLeafId(next)`). With three panes, closing the right one focuses the far left. `settleFocus` would have chosen MRU but this assignment pre-empts it.
- Editor tabs of the closed pane are appended to the survivor (`view-ops.ts:transferTabs`): tabs of workspace A attach to a chat of workspace B and `editorOpen` is forced true on the survivor. Intent: tabs follow their workspace's chat, or close with a prompt (OWNER).
- Last chat of a tab closed: the active tab becomes the next view of the same project AND workspace set, else the empty stage (`view-ops.ts:dropRecord` ~330 and its comment "the route names that workspace"). Closing a tab can therefore show an empty stage while other tabs exist. Intent: show the adjacent tab.
- Unsaved tabs survive only when the closed pane is the view's last leaf (`view-ops.ts:keepUnsavedTabs` ~412); in a view with chatless leaves removed by `removeView` unsaved tabs would be dropped. Reachable only through dead code today (`splitPane`), but the guard is incomplete.
- `mod+w` with no editor tabs closes a CHAT pane when its tree has two or more panes (`hooks/use-pane-keyboard.ts` TAB_CLOSE ~146-168): the user pressed "close tab" and lost a conversation; with one pane it does nothing. Intent (OWNER): `mod+w` closes the active tab; closing a split pane is the handle's job.
- `closePane` releases the chat after the state write, in a second step (`view-actions.ts:closePane` ~372-379): two writes, so a failure leaves a closed pane with a live chat runner.

## F5. Detach and merge (pane handle, tab drag)

Intent: "Open in its own tab" moves that chat to a new tab right of its group and activates it; dragging a chat onto another tab merges it into that tab; reordering moves whole tabs; no chat is ever lost or duplicated.

Observed bugs:
- `detachPane` returns silently when the view has fewer than two chats (`view-actions.ts:detachPane` ~359) and merge/detach live in sidebar code (`components/sidebar/lib/pane-drop-actions.ts:reorderRecentsEntries`), splitting one concept across two modules. A partial drag of a group first detaches then reorders as two undoable-less steps (~60-64).
- Detach activates the new view only when the source was showing (`view-actions.ts` ~368), else the moved chat is hidden.
- `movePane` across views leaves `pane.viewId` update to the caller order; between the `writeLayout` calls the invariant "pane is a leaf of exactly one tree" is momentarily false (`view-ops.ts:movePane` ~508-513); only the dev-time integrity checker (`stores/window-pane-store.ts:store.subscribe`, DEV only) notices.

## F6. Focus rules

Intent: clicking anywhere in a pane focuses it, including its buttons; exactly one pane is active; the active ring appears only with 2+ panes; after any structural change focus is deterministic (see F4).

Observed bugs:
- Activation skips direct hits on `button, input, textarea, [role=button], [role=menu]` (`hooks/use-pane-activation.ts` ~19-22), then a separate `onClick` on the container activates anyway (`components/pane-container.tsx:handlePaneClick`): two mechanisms for one rule, and a button that stops propagation never activates. Rust: one rule (any press activates, except popups).
- `focusPane` on a stage pane while a view is active switches the screen to the stage (`lib/view-state.ts:focusPane` ~255); intent: stage is empty state only.
- The ring is hidden for a lone pane, so single-pane focus has no indicator, even when the focus is in the chat versus the editor half.
- Pane-navigate keys move to the first leaf of the other child of the outermost matching split (`utils/pane-layout.ts:findAdjacent` ~157-170), not the geometric neighbour. Intent: geometric neighbour in the direction, wrapping never; with no neighbour do nothing.

## F7. Close, restore and reopen

Intent: closing a tab/pane can be undone; reopen restores what was closed (chat pane, its position, its editor tabs); app restart restores the same layout, active tab, active pane, ratios.

Observed bugs:
- Only closed FILE tabs can be reopened (`stores/slices/buffer-slice.ts:reopenLastClosedBuffer` ~266-276 via `closedBuffersHistory`), into whichever pane is active; closed chats, panes, views, terminals and browser tabs have no reopen. `mod+shift+t` therefore does nothing after closing a chat tab.
- On restart the saved layout is repaired by dropping violating panes (`lib/view-repair.ts:repairViewState`): a bad pane silently disappears, a chat gone from the daemon leaves the layout (`placeRestoredMembers`). No user message.
- Saved fields are listed twice by hand: the persistence subscriber compares an explicit field list and its own comment warns "A field left out of this comparison silently never persists" (`stores/window-pane-store.ts` ~204). `activeProjectId` and `fullscreenPaneId` are not saved.
- Legacy shapes are still accepted on load (`newTab` placeholder, `sourceFilePath`, buffers with text; `utils/persisted-layout.ts`). The Rust side starts at the current format; migrate once on import.

## F8. Chat / editor halves

Intent: the editor area opens beside the chat when there is room; a tab opening into a chat-only pane opens the area; the choice persists per pane; terminals and webviews never lose state when the layout changes.

Observed bugs:
- `togglePaneEditor` writes via `windowPaneStore.setState` directly (`utils/pane-command-actions.ts:togglePaneEditor` ~70-76) instead of the pane-action path that settles focus and releases buffers; its own file rule says every pane write goes through `commitViewWrite`.
- `editorOpen=false` hides the editor even when tabs exist (state D collapses to tabs rather than chat-only), while closing the last tab also sets it false: two meanings for one flag (`hooks/use-pane-presentation.ts`, `pane-actions/editor-tab-actions.ts:126`).
- Opening a tab respects a user's earlier "off" only when the pane already had tabs (`editor-tab-actions.ts:placeTab` ~64): hidden state, depends on history.
- A terminal reopened from another pane focuses the existing pane instead of moving (`stores/slices/buffer-slice.ts:openContent` ~118-125); fine as intent, but the "New Terminal" button on a second pane then appears to do nothing.

## F9. Console dock

Intent: toggle shows the log; overlay closes on outside click; push resizes the app; size, dock, mode persist.

Observed bugs: none functional. Outside-press close uses a capture listener that excludes the toggle by selector (`features/console/components/console-panel.tsx` ~59, `aria-controls`): coupling by string. Scroll-follow depends on a 250 ms user-activity window timer (`console-log.tsx` `USER_SCROLL_WINDOW_MS`).

## Workarounds and suspected bugs (checklist for the Rust side: do not copy)

| # | Item | Where | Kind | Rust guidance |
|---|---|---|---|---|
| 1 | 10 px / 10 percent dual minimum | `constants/pane.ts`, `pane-sash.tsx`, `pane-layout.ts:normalizeSizes` | inconsistent invariant | one minimum |
| 2 | Pre-empting `settleFocus` with first-leaf survivor | `view-ops.ts:removePane` | wrong focus | pick neighbour/MRU |
| 3 | `settleFocus` after every write | `view-ops.ts:commitViewWrite` | resync | focus must be a derived field, set once |
| 4 | `viewIntegrityViolations` (DEV) and `repairViewState` on load | `lib/view-integrity.ts`, `lib/view-repair.ts` | detector for writers that can break invariants | make invalid states unrepresentable; keep a single mutator |
| 5 | Stage doubles as first tab, id reuse in `promoteStage` | `view-ops.ts` ~335 | modelling hack | explicit empty state |
| 6 | Three trees (views, stage, bottom) and `PaneHome` branching | `view-state.ts` | dead bottom tree | drop bottom |
| 7 | Fullscreen overlay and `fullscreenPaneId` still wired | `split-view-root.tsx`, `use-browser-occluded.ts` | dead branch | drop |
| 8 | `PaneAccentRing` separate overlay, `data-pane-resizing` flattening, edge-dissolve pause | `pane-accent-ring.tsx`, `index.css` | WebKit performance workarounds | not needed natively |
| 9 | Idle-task staged mounting of parked views and `pendingChatSurfaces` | `hooks/use-mounted-views.ts`, `stores/pending-chat-surfaces.ts` | timer-based sequencing | webview pool by visibility |
| 10 | 300 ms debounced save | `window-pane-store.ts` ~205 | timer | save on change, coalesced by the executor |
| 11 | 50 ms frame batching and `requestAnimationFrame` pinning in console | `console/lib/controller.ts`, `console-log.tsx` | timers | virtual list natively |
| 12 | Re-seed state during render when props change | `use-chat-presentation.ts` ~74-111 (react-doctor suppressions) | workaround for React | derived state |
| 13 | Two activation paths (capture mousedown + onClick) | `use-pane-activation.ts`, `pane-container.tsx` | redundancy | one |
| 14 | Overlay shown for drags that cannot split (tab, file) | `use-pane-drop-handlers.ts`, `drag-store.ts` | misleading UI | only for chat drags |
| 15 | `splitPane`, `createPaneBeside`, `getOrCreatePaneDropTarget`, `distributePaneSplit`, `setPaneLocked`, `moveEditorTabToPane` | see port list | dead | drop |
| 16 | Hidden `inert` / `display:none` parked views | `split-view-root.tsx` | DOM trick | park explicitly |
| 17 | `EdgeDissolve` 7 backdrop-filter layers | `components/ui/edge-dissolve.tsx` | emulating progressive blur | native blur or gradient |
| 18 | `isBottomPane` props passed through every header component | `tabs/components/*` | dead plumbing | drop |
| 19 | Pane crash `PaneBoundary` | `pane-boundary.tsx` | React isolation | pane host isolates frames |
| 20 | Legacy-format upgrade paths | `persisted-layout.ts` | migration | one-time import |
