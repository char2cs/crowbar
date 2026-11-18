# 8. Split panes (SPL)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| SPL-001 | Create split: drag view tab 2 onto right edge zone of the showing pane, release; chat joins as right pane, both in one view tab | web/src/features/panes/stores/slices/pane-actions/chat-drop-actions.ts:35 | rig reset; drag tab 2 onto right quarter of the pane | — | | | | | | | |
| SPL-002 | Drop on left edge zone: new pane before (left) | web/src/features/panes/utils/pane-drop-zones.ts:20 | drag tab 2 onto left quarter of the pane | — | | | | | | | |
| SPL-003 | Drop on top edge zone: vertical split, new pane on top | web/src/features/panes/utils/pane-drop-zones.ts:22 | drag tab 2 onto top quarter of the pane | — | | | | | | | |
| SPL-004 | Drop on bottom edge zone: vertical split, new pane below | web/src/features/panes/utils/pane-drop-zones.ts:23 | drag tab 2 onto bottom quarter of the pane | — | | | | | | | |
| SPL-005 | Drop on centre of an occupied pane: always adds on the right | web/src/features/panes/stores/slices/pane-actions/chat-drop-actions.ts:33 | drag tab 2 onto pane middle | — | | | | | | | |
| SPL-006 | Drop chat on centre of a chatless pane: fills it in place; an already-open chat is focused instead | web/src/features/panes/stores/slices/pane-actions/chat-drop-actions.ts:20 | close both tabs (stage); drag a chat's sidebar row onto stage middle | — | | | | | | | |
| SPL-007 | Zone preview while dragging over a pane: secondary 2px border, secondary/14 fill, rounded-lg, half-pane rectangle inset 4px (full pane for centre) | web/src/features/panes/components/split-drop-overlay.tsx:67 | drag tab 2 over the pane, hold in each zone | transition-all 100ms (default ease) | | | | | | | |
| SPL-008 | Zone preview moves between zones (diagonal quadrants, 25% band) | web/src/features/panes/utils/pane-drop-zones.ts:18 | drag tab 2 slowly across the pane left → right → top → bottom | transition-all 100ms | | | | | | | |
| SPL-009 | Hovered pane ring during a sidebar-row drag (when no zone overlay): ring-2 secondary | web/src/features/panes/components/pane-container.tsx:208 | drag a sidebar chat row over a pane edge (see SB) | — | | | | | | | |
| SPL-010 | Dragging a chat already open in another view: moves its pane into the split; old view tab disappears | web/src/features/panes/stores/slices/pane-actions/chat-drop-actions.ts:40 | rig reset; drag tab 2 onto pane | — | | | | | | | |
| SPL-011 | Drop a split member onto its own pane: no-op | web/src/features/panes/stores/slices/pane-actions/chat-drop-actions.ts:41 | in a split, drag a pane grip onto the same pane | — | | | | | | | |
| SPL-012 | Cross-project drop refused with toast "That chat belongs to a different space" | web/src/components/sidebar/lib/pane-drop-actions.ts:139 | tab list scope 'all'; drag other project's tab onto pane. Seed: second project with open chat | toast (see TST) | | | | | | | |
| SPL-013 | Horizontal split rendering: two panes with gutter + sash, default 50/50 flex basis | web/src/features/panes/components/pane-node-renderer.tsx:83 | make SPL-001 split | — | | | | | | | |
| SPL-014 | Nested split (split a pane of a split) | web/src/features/panes/components/pane-node-renderer.tsx:76 | make SPL-001, then drag a third chat onto bottom zone of right pane. Seed: a third chat | — | | | | | | | |
| SPL-015 | Pane handle: primary-coloured tab top-centre, rounded-b-md, shown on pane hover or focus-within, only in a split | web/src/features/panes/components/pane-handle.tsx:58 | in a split, hover a pane | opacity 150ms cubic-bezier(0.4,0,0.2,1) | | | | | | | |
| SPL-016 | Handle grip "Drag this chat": pointer drag of the pane's chat (cursor grab), same drop rules as a view tab | web/src/features/panes/components/pane-handle.tsx:63 | in a split, drag grip onto other pane's top zone | — | | | | | | | |
| SPL-017 | Handle "Open in its own tab": pane leaves the split, becomes its own view tab | web/src/features/panes/stores/slices/pane-actions/view-actions.ts:95 | in a split, hover pane, click middle handle button | — | | | | | | | |
| SPL-018 | Handle "Close this chat": pane closes; its editor tabs go to the surviving pane | web/src/features/panes/lib/view-ops.ts:126 | in a split, open a file in one pane, click its × handle button | — | | | | | | | |
| SPL-019 | Handle button tooltips (bottom side) | web/src/features/panes/components/pane-handle.tsx:64 | hover each handle button | fade/zoom-in-95 150ms after 150ms delay | | | | | | | |
| SPL-020 | Grip drag into the tab strip detaches into a new view tab (see VT-051) | web/src/components/sidebar/lib/tab-strip-drag.ts:263 | in a split, drag grip up into strip, release | transform 150ms ease-out | | | | | | | |
| SPL-021 | Sash hover: bg-border/60, col-resize (horizontal split) / row-resize (vertical) | web/src/features/panes/components/pane-sash.tsx:200 | in a split, hover the seam | transition-colors 150ms | | | | | | | |
| SPL-022 | Sash drag resizes live; panes go flat (radius/shadow off, header blur off) until release | web/src/index.css:142 | in a split, drag seam left | — | | | | | | | |
| SPL-023 | Sash clamps each side at 10px (MIN_PANE_SIZE) | web/src/features/panes/constants/pane.ts:1 | drag seam to window edge | — | | | | | | | |
| SPL-024 | Sash double-click: no reset (no handler) | web/src/features/panes/components/pane-sash.tsx:191 | double-click seam | — | | | | | | | |
| SPL-025 | Sash is not keyboard operable (role separator, no tabindex/keys) | web/src/features/panes/components/pane-sash.tsx:194 | Tab through a split | — | | | | | | | |
| SPL-026 | Split sizes persist in the layout and restore on restart | web/src/features/panes/stores/window-pane-store.ts:84 | resize split; rig restart tauri | — | | | | | | | |
| SPL-027 | Active pane accent ring (only with >1 visible pane); moves on click | web/src/features/panes/components/pane-accent-ring.tsx:44 | in a split, click the other pane | opacity 150ms (default ease) | | | | | | | |
| SPL-028 | ctrl+alt+Left/Right/Up/Down focuses the adjacent pane | web/src/features/panes/hooks/use-pane-keyboard.ts:182 | in a split, ctrl+alt+Right | ring opacity 150ms | | | | | | | |
| SPL-029 | ctrl+w in a split pane with no editor tabs: closes that pane | web/src/features/panes/hooks/use-pane-keyboard.ts:158 | in a split, focus a pane with no files; ctrl+w | — | | | | | | | |
| SPL-030 | Last remaining member: split collapses to a single pane, handle and accent ring disappear | web/src/features/panes/lib/view-ops.ts:180 | in a split, close one member | — | | | | | | | |
| SPL-031 | Each split member keeps its own header, editor and chat/editor sash | web/src/features/panes/components/pane-container.tsx:235 | in a split, Open editor in one pane | — | | | | | | | |
| SPL-032 | Narrow split member drops to tabs presentation (pane box max side <780) | web/src/features/agent/hooks/use-chat-presentation.ts:190 | in a split at rig size standard, Open editor in one pane | — | | | | | | | |
| SPL-033 | File dragged from Files sidebar over a pane: zone preview shown, release opens the file as a tab in that pane (no split) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-drag-drop.ts:211 | drag README.md from Files onto a pane edge | transition-all 100ms | | | | | | | |
| SPL-034 | OS file drag over a pane: ring-2 secondary + secondary/10 wash | web/src/features/panes/components/pane-container.tsx:217 | drag a file from the desktop onto the window | — | | | | | | | |
| SPL-035 | Hidden (parked) views stay mounted, display:none and inert | web/src/features/panes/components/split-view-root.tsx:71 | split view showing; click another view tab and back | — | | | | | | | |
| SPL-036 | Light / dark split chrome (pane borders, gutter, accent) | web/src/features/panes/utils/pane-border.ts:31 | rig theme light; rig theme dark | — | | | | | | | |

Skipped:
- Split/close/reset/move/lock editor-group commands (web/src/features/panes/utils/pane-command-actions.ts:39-234, incl. `distributePaneSplit` caller at 197): no callers, no menu, no chord.
- Fullscreen / maximize pane overlay (web/src/features/panes/components/split-view-root.tsx:88-99): `togglePaneFullscreen` has no caller.
- HTML5 tab-drop split (web/src/features/panes/hooks/use-pane-drop-handlers.ts:62-81): `application/tab-data` is never set.
- Bottom pane split (BOTTOM_PANE_ID): `setIsBottomPaneVisible` has no caller.
