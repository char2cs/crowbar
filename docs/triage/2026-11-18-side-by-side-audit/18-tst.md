# 18. Toasts (TST)

### Toast system (base-ui Toast, web-drawn)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-001 | Position, sidebar open: inside the sidebar column's bottom, full sidebar width, p-2, gap 2, newest at the bottom | web/src/components/layout/sidebar-toast-overlay.tsx:119 | Settings > Developer > Export settings (TST-032) with the sidebar open | — | | | | | | | |
| TST-002 | Position, sidebar hidden: fixed bottom-4, left-4 (right-4 when the sidebar is on the right), w-72, newest on top | web/src/components/layout/sidebar-toast-overlay.tsx:134 | hide the sidebar; trigger TST-032; then `rig sidebar right` and repeat | — | | | | | | | |
| TST-003 | Sidebar on the right: viewport follows it; swipe directions right/down instead of left/down | web/src/components/layout/sidebar-toast-overlay.tsx:113 | `rig sidebar right`; trigger TST-032 | — | | | | | | | |
| TST-004 | Stacking, sidebar open: at most 3 shown; pinned (timeout 0) toasts never evicted, transient ones fill the rest newest-first | web/src/components/layout/sidebar-toast-overlay.tsx:38 | click Export settings 4 times quickly; count cards | — | | | | | | | |
| TST-005 | Stacking, sidebar hidden: every toast in the list is drawn (no 3-cap window) | web/src/components/layout/sidebar-toast-overlay.tsx:138 | hide the sidebar; click Export settings 4 times quickly | — | | | | | | | |
| TST-006 | Card: rounded-lg popover card, type icon (error WarningCircle red, info Info, success CheckCircle green, warning Warning amber, loading CircleNotch spinning), title medium, description muted, X close top-right (50% → 100% on hover), optional xs action button bottom-right | web/src/components/layout/sidebar-toast-overlay.tsx:66 | trigger TST-032 (success), TST-025 (info), TST-040 (error), TST-048 (loading); compare | — | | | | | | | |
| TST-007 | Auto-dismiss: base-ui default 5000ms unless the call sets `duration`; `duration: 0` persists until dismissed by code | web/src/features/window/stores/toast-store.ts:19 | trigger TST-032; time it to removal | — | | | | | | | |
| TST-008 | Hover pause: timers pause while the viewport is hovered or focused, and while the window is unfocused | web/src/components/layout/sidebar-toast-overlay.tsx:119 | trigger TST-032; hover the card for 8s; move away | — | | | | | | | |
| TST-009 | Dismiss by X | web/src/components/layout/sidebar-toast-overlay.tsx:104 | trigger TST-032; click X | exit fade 200ms | | | | | | | |
| TST-010 | Dismiss by swipe (left or right per side, or down) | web/src/components/layout/sidebar-toast-overlay.tsx:68 | trigger TST-032; drag the card left past the threshold | — | | | | | | | |
| TST-011 | Enter/exit animation of every toast | web/src/components/layout/sidebar-toast-overlay.tsx:69 | trigger TST-032; watch in and out | opacity 0↔1, 200ms cubic-bezier(0.4,0,0.2,1); no slide or scale | | | | | | | |
| TST-012 | Same key replaces in place instead of stacking: connection toasts share one key; report() keys by action title; operation failures by op id | web/src/lib/errors/report.ts:57 | click Export settings twice (no key: stacks) vs. Files > Reveal in Finder twice (TST-110, keyed by action: one card) | — | | | | | | | |
| TST-013 | Toasts raised before a viewport mounts (boot) are held and shown when the shell mounts | web/src/components/ui/toast.tsx:136 | `rig reset` with a boot failure. Seed: failing session restore | — | | | | | | | |
| TST-014 | Loading icon keeps spinning under reduced motion | web/src/components/layout/sidebar-toast-overlay.tsx:79 | reduced motion on; trigger TST-048. Seed: queued prompt on a running agent | — | | | | | | | |
| TST-015 | report() stays silent for a cancelled request, an operation failure (the watcher toasts it) and a migrating/halted daemon refusal | web/src/lib/errors/report.ts:49 | not drivable alone | — | | | | | | | |

### Connection and backend

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-016 | warning "Backend unavailable" / "Reconnecting…", pinned, after 500ms of disconnect | web/src/components/layout/connection-indicator.tsx:66 | stop the daemon. Seed: daemon stopped | — | | | | | | | |
| TST-017 | success "Reconnected" / "Backend connection restored." replaces it, only if the warning was shown | web/src/components/layout/connection-indicator.tsx:54 | restart the daemon after TST-016. Seed: daemon restart | — | | | | | | | |
| TST-018 | info "Crowbar is upgrading its data" / "Step i of n: <name>" (or the daemon's own message), pinned, updates in place | web/src/components/layout/connection-indicator.tsx:19 | Seed: daemon running a data migration | — | | | | | | | |
| TST-019 | warning "Backend unresponsive" / "Capturing a diagnostics dump and restarting it…" | web/src/features/window/components/daemon-health-listener.tsx:40 | Seed: app-supervised daemon that hangs | — | | | | | | | |
| TST-020 | success "Backend restarted" / "Sessions reconnect automatically." | web/src/features/window/components/daemon-health-listener.tsx:50 | after TST-019. Seed: as TST-019 | — | | | | | | | |
| TST-021 | success "Diagnostics copied"; error "Could not copy diagnostics" | web/src/features/window/components/daemon-crash-panel.tsx:35 | crash panel > Copy diagnostics. Seed: daemon crash loop | — | | | | | | | |
| TST-022 | error "Could not open the log" / <reason> | web/src/features/window/components/daemon-crash-panel.tsx:10 | crash panel > Open log. Seed: daemon crash loop with no log | — | | | | | | | |
| TST-023 | error "<label> failed" / <reason>, keyed by operation, once per failure seen live | web/src/components/layout/operations-watcher.tsx:39 | Seed: failing daemon operation (e.g. Retry setup on a broken branch) | — | | | | | | | |
| TST-024 | error "Couldn't set up <branch>" / placeholder reason, keyed by workspace, with action button "Fix…" that opens the Detach dialog; skipped when an operation failure already covers it | web/src/components/layout/placeholder-toast-watcher.tsx:34 | Sidebar > unprovisioned branch row appears. Seed: branch held by a checkout outside Crowbar | — | | | | | | | |

### Sidebar and rows

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-025 | info "No image on the clipboard" | web/src/features/agent/composer/composer-plus-button.tsx:71 | open Perf fixture chat; composer "+" > Paste image with an empty clipboard | — | | | | | | | |
| TST-026 | error "Could not fetch the GitHub avatar" / "Check that the repo has a GitHub origin remote and the gh CLI is installed and authenticated." | web/src/components/layout/icon-popover.tsx:158 | Sidebar > click the checkout repo icon > GitHub | — | | | | | | | |
| TST-027 | error "Start a thread from a workspace row — a folder has none to run it in" | web/src/components/layout/create-actions.ts:115 | create a folder (row menu > New folder); hover it > New thread | — | | | | | | | |
| TST-028 | error "Can't delete <repo> yet" (Delete Repo held nothing) | web/src/components/sidebar/lib/row-menu.ts:249 | Seed: repo whose removal plan is empty (e.g. already removing) | — | | | | | | | |
| TST-029 | error "Can't remove <row>" | web/src/components/sidebar/lib/row-menu.ts:270 | hover a locked branch's Remove. Seed: row the trash path refuses | — | | | | | | | |
| TST-030 | error "Can't delete <row> yet" (drag to trash) / "Can't delete this space — it may already be gone" | web/src/components/layout/sidebar-tree-surface.tsx:88 | Seed: trash refused for a row or a missing space | — | | | | | | | |
| TST-031 | info "Also removed N chat(s)" / their titles, after a chat delete cascades | web/src/components/layout/removal-commit.ts:154 | Seed: chat whose delete takes sibling chats with its branch | — | | | | | | | |
| TST-032 | success "Settings exported" / "Saved crowbar-settings.json to your downloads." | web/src/features/settings/components/tabs/developer-settings.tsx:26 | Sidebar header > settings gear > Developer > Export settings | — | | | | | | | |
| TST-033 | error "Can't <verb> <branch> yet — its chat is still loading" (open, move, rename, lock, unlock) | web/src/components/sidebar/lib/row-actions.ts:24 | act on a branch row right after start. Seed: slow-loading owning chat | — | | | | | | | |
| TST-034 | error "That chat belongs to a different space" | web/src/components/sidebar/lib/pane-drop-actions.ts:151 | drag a chat from another space onto a pane. Seed: second space with a chat | — | | | | | | | |
| TST-035 | error "Can't move <row> there" | web/src/components/sidebar/lib/sidebar-drop-dispatch.ts:69 | Seed: drop the daemon refuses | — | | | | | | | |
| TST-036 | error "Can't start a new thread yet" (home workspace not known) | web/src/components/sidebar/space-scroller.tsx:82 | New thread right after start. Seed: space whose home workspace is still loading | — | | | | | | | |
| TST-037 | error "No agent provider is enabled" / "Enable one in Settings → Providers to start a chat or fork a workspace." | web/src/components/layout/thread-create.ts:105 | Settings > Providers: disable all; `rig key <app> ctrl+n` | — | | | | | | | |
| TST-038 | error "No agent provider is enabled on <host>" / "Enable one on <host> itself…" | web/src/components/layout/thread-create.ts:113 | Seed: remote host with no enabled provider | — | | | | | | | |
| TST-039 | error "Couldn’t reach <host>" / "Check that it is online in Settings → Hosts." (new thread) | web/src/components/layout/thread-create.ts:147 | Seed: offline remote host | — | | | | | | | |

### Chat and agent

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-040 | error "<Provider> isn’t installed" / "Crowbar couldn’t find the <Provider> command on this machine. Install it and make sure it’s on your PATH, then try again." | web/src/features/agent/lib/spawn-error.ts:32 | open Perf fixture chat; type `hi`; Return (no provider CLI installed) | — | | | | | | | |
| TST-041 | error "Can’t <start\|resume\|switch> <Provider> while the chat is working" / "Wait for the current turn to finish, or stop it, then try again." | web/src/features/agent/lib/spawn-error.ts:40 | switch provider mid-turn. Seed: running agent | — | | | | | | | |
| TST-042 | error "The session was lost" / "Start a new conversation to continue." | web/src/features/agent/lib/spawn-error.ts:47 | Seed: chat whose provider session is gone | — | | | | | | | |
| TST-043 | error "Couldn’t <action> <Provider> chat" / daemon words or "Crowbar could not reach the daemon — try again." | web/src/features/agent/lib/spawn-error.ts:52 | Seed: spawn failing for another reason | — | | | | | | | |
| TST-044 | success "Copied to clipboard"; error "Could not copy to clipboard" (transcript message copy) | web/src/features/agent/transcript/rows/text-row.tsx:17 | Perf fixture chat > hover a message > copy | — | | | | | | | |
| TST-045 | info "The agent is busy" / "End the shell once the current turn finishes." | web/src/features/agent/chat/lib/offered-stop-shell.ts:14 | Seed: running agent with an offered shell stop | — | | | | | | | |
| TST-046 | error "Could not set permission level" / "Crowbar could not reach the daemon — try again." | web/src/features/agent/composer/permission-level-switcher.tsx:79 | Seed: daemon down while switching permission level | — | | | | | | | |
| TST-047 | error "Its provider already has this prompt" / "Stop ends its turn." | web/src/features/agent/stores/chat-outbox.ts:206 | Esc to take back a prompt already delivered. Seed: running agent | — | | | | | | | |
| TST-048 | loading "Taking the prompt back…", pinned until the daemon answers | web/src/features/agent/stores/chat-outbox.ts:221 | Esc on a queued prompt. Seed: running agent with a queued prompt | — | | | | | | | |

### Editor, files and terminal

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-049 | warning "<file> changed on disk" / "Your unsaved changes are kept. Saving will overwrite the on-disk version." | web/src/features/workspace/lib/external-buffer-sync.ts:55 | open README.md, type a character; `rig key <app> ctrl+j`; `rig type <app> echo x >> README.md`; Return | — | | | | | | | |
| TST-050 | warning "Not following changes to <file>" / why the watch was refused | web/src/features/workspace/lib/external-buffer-sync.ts:116 | Seed: host document whose watch the daemon refuses | — | | | | | | | |
| TST-051 | error "<title>" / "<message>" for documents that fail to reconcile on workspace activation | web/src/features/workspace/lib/external-buffer-sync.ts:182 | Seed: open file deleted while its workspace was hidden | — | | | | | | | |
| TST-052 | error "Workspace is read-only" / "Create or switch to a child workspace to edit files." | web/src/features/editor/lib/buffer-save.ts:65 | Seed: read-only workspace; edit a file; `rig key <app> ctrl+s` | — | | | | | | | |
| TST-053 | error "Save or discard changes in <file> first" (Save as onto an open dirty file) | web/src/features/editor/lib/buffer-save.ts:155 | open README.md, type a character; `rig key <app> ctrl+shift+n`; type; `rig key <app> ctrl+s`; answer `README.md` | — | | | | | | | |
| TST-054 | error open failures: "File not found" / path; "Can't open file: permission denied"; "File too large to open"; "Failed to open file"; "Can't open this file" / "It holds Crowbar's own state."; "Can't watch this file: too many file watches" | web/src/features/editor/documents/open-document.ts:99 | Open file… (ctrl+o) a file the daemon refuses. Seed: unreadable, oversized or state file | — | | | | | | | |
| TST-055 | error per failure kind after restoring tabs: "<title>" / comma list of files (workspace in parentheses when twins) | web/src/features/editor/documents/restore-documents.ts:267 | `rig reset` with a tab whose file was deleted. Seed: restored tab of a missing file | — | | | | | | | |
| TST-056 | error "Definition is outside this workspace" / path | web/src/features/editor/lsp/monaco-lsp-providers.ts:311 | Go to definition into a dependency. Seed: language server installed | — | | | | | | | |
| TST-057 | error "Rich markdown editor failed" / "Switched to source view for this file." | web/src/features/editor/markdown/plate/markdown-editor-pane.tsx:58 | Seed: markdown that crashes the rich editor | — | | | | | | | |
| TST-058 | error "Paste failed" / failed destination paths | web/src/features/file-explorer/file-explorer/hooks/use-tree-container-events.ts:48 | Files > Copy a file; select a folder; ctrl+v. Seed: paste the daemon refuses | — | | | | | | | |
| TST-059 | error "Terminal unavailable" / "Could not start the terminal. Try reopening the tab." | web/src/features/terminal/hooks/use-terminal-attachment.ts:114 | Seed: PTY spawn failure | — | | | | | | | |
| TST-060 | error "Cannot open file" / "<path> is outside the current workspace." | web/src/features/terminal/lib/terminal-links.ts:307 | `rig key <app> ctrl+j`; `rig type <app> echo /etc/hosts`; Return; ctrl+click the path | — | | | | | | | |
| TST-061 | error "Cannot open file" / "Could not resolve <text> to a path." | web/src/features/terminal/lib/terminal-links.ts:318 | in a terminal print `src/missing.ts:3`; ctrl+click it | — | | | | | | | |
| TST-062 | error "Could not copy link" / URL | web/src/features/terminal/lib/terminal-url-menu.ts:51 | Seed: clipboard write failure | — | | | | | | | |
| TST-063 | success "Copied to clipboard"; error "Could not copy to clipboard" (review comment) | web/src/features/git/components/review-message-row.tsx:29 | review thread on src/pricing.ts > "..." > Copy as Markdown | — | | | | | | | |
| TST-064 | success "Pointer copied" / "<n> items" when more than one | web/src/features/browser/hooks/use-pointer-pick.ts:26 | browser tab > pick element (ctrl+shift+e) > click. Seed: browser tab with a loaded page | — | | | | | | | |
| TST-065 | info "This link is on <host>, not this computer — forward the port with ssh -L to open it here" | web/src/lib/workspace-link.ts:32 | open a localhost link from a remote workspace. Seed: remote workspace | — | | | | | | | |

### Settings and hosts

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-066 | success "Settings imported" / "Your settings have been restored from the file." | web/src/features/settings/components/tabs/developer-settings.tsx:39 | Settings > Developer > Import settings > pick a valid export. Seed: settings export file on disk | — | | | | | | | |
| TST-067 | error "Import failed" / "The file is not a valid Crowbar settings export." or "Could not read the selected file." | web/src/features/settings/components/tabs/developer-settings.tsx:41 | Import settings > pick a non-settings JSON. Seed: invalid JSON file on disk | — | | | | | | | |
| TST-068 | success "Settings reset" / "All settings have been restored to their defaults." | web/src/features/settings/components/tabs/developer-settings.tsx:60 | Settings > Developer > Reset all > Reset all | — | | | | | | | |
| TST-069 | success "Diagnostics exported" / bundle path | web/src/features/settings/components/tabs/developer-settings.tsx:137 | Settings > Developer > Export diagnostics | — | | | | | | | |
| TST-070 | success "Browsing data cleared" / "Reload open pages to see the change." (macOS 14+ only) | web/src/features/settings/components/tabs/browser-settings.tsx:31 | macOS: Settings > Browser > Clear data > Clear data | — | | | | | | | |
| TST-071 | success "Agent added" / "<id> is now available as a provider."; error "Could not refresh agents" / "Reopen this tab to load the new provider." | web/src/features/settings/components/tabs/providers-settings.tsx:30 | Settings > Providers > add agent file. Seed: agent definition file | — | | | | | | | |
| TST-072 | error "Could not save providers" / "Crowbar could not reach the daemon — try again." | web/src/features/settings/hooks/use-providers-settings.ts:83 | Seed: daemon down while toggling a provider | — | | | | | | | |
| TST-073 | error "Could not load permission level" / "Could not save permission level" + daemon-unreachable text | web/src/features/settings/components/tabs/default-permission-level-setting.tsx:38 | Seed: daemon down on Settings > Chat permission level | — | | | | | | | |
| TST-074 | info "Removed <host>" / "Its pairing was already revoked or lost…" | web/src/features/hosts/lib/host-revoke.ts:38 | Settings > Hosts > Revoke a host that already dropped the pairing. Seed: such a host | — | | | | | | | |
| TST-075 | error "Removed <host>, but couldn’t clear everything this app kept for it" / reason | web/src/features/hosts/lib/host-revoke.ts:112 | Seed: keychain/token cleanup failure on revoke | — | | | | | | | |
| TST-076 | error host failures for any action: "Couldn’t reach <host>"; "<host> is presenting a different identity — confirm it before pairing again"; "<host> needs to be paired again"; "<host> returned an error (<status>)" | web/src/features/hosts/lib/host-failure-notice.ts:101 | Seed: remote host offline, re-keyed, unpaired or erroring | — | | | | | | | |
| TST-077 | error project-host notices: "This project’s host is not available" / "Check the host in Settings → Hosts."; "That host is no longer paired with this computer"; "This project is no longer in your list"; "Still loading your projects" / "Try again in a moment." | web/src/features/hosts/lib/host-failure-notice.ts:19 | Seed: project on an unavailable or removed host | — | | | | | | | |

### report() error toasts (title = action; description = host words, daemon words, "Something went wrong — details are in the log" or the fallback)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| TST-078 | "Something went wrong" (any unhandled promise rejection) | web/src/lib/errors/report.ts:68 | Seed: unhandled rejection | — | | | | | | | |
| TST-079 | "Restore the last session"; "Watch the window close" | web/src/app-main.tsx:130 | Seed: failing session restore at boot | — | | | | | | | |
| TST-080 | "Create workspace" | web/src/components/layout/create-actions.ts:201 | Fork branch. Seed: fork the daemon refuses | — | | | | | | | |
| TST-081 | "Failed to create space"; "Failed to set icon" (new space) | web/src/components/layout/create-space-panel.tsx:115 | Seed: space create refused | — | | | | | | | |
| TST-082 | "Failed to set icon"; "Failed to set emoji"; "Failed to reset icon" | web/src/components/layout/icon-popover.tsx:89 | Seed: icon write refused | — | | | | | | | |
| TST-083 | "Could not set permission level" (draft thread first send) | web/src/components/layout/draft-thread.ts:75 | Seed: daemon refuses permission level | — | | | | | | | |
| TST-084 | "Remove <label>"; "Remove"; "Couldn't remove <label>" (fallback "request failed"); "Dismiss" | web/src/components/layout/removal-commit.ts:77 | Seed: removal the daemon refuses | — | | | | | | | |
| TST-085 | "Failed to add repository"; "Failed to import project" | web/src/components/projects/add-repository-modal.tsx:127 | Seed: add-repo refused | — | | | | | | | |
| TST-086 | "Move"; "Cancel move" | web/src/components/sidebar/lib/drop-actions.ts:123 | Seed: move refused | — | | | | | | | |
| TST-087 | "Failed to rename branch"; "Failed to rename folder"; "Failed to rename chat"; "Failed to rename repository"; "Failed to rename project" | web/src/components/sidebar/lib/row-actions.ts:59 | Seed: rename refused | — | | | | | | | |
| TST-088 | "Failed to update lock" | web/src/components/sidebar/lib/row-actions.ts:219 | Seed: lock refused | — | | | | | | | |
| TST-089 | "Failed to create folder" | web/src/components/sidebar/lib/row-actions.ts:249 | Seed: folder create refused | — | | | | | | | |
| TST-090 | "Dismiss" (operation row) | web/src/components/sidebar/operation-sidebar-row.tsx:99 | Seed: failed operation row whose dismiss fails | — | | | | | | | |
| TST-091 | "Stream"; "Stream <scope>" | web/src/core/stream/host-stream.ts:183 | Seed: stream subscription refused | — | | | | | | | |
| TST-092 | "Couldn’t compact the chat"; "Couldn’t end the shell" | web/src/features/agent/chat/lib/offered-compact.ts:10 | Seed: compaction refused | — | | | | | | | |
| TST-093 | "Couldn’t keep the prompt you took back" / "The saved prompts are full or unavailable." | web/src/features/agent/chat/use-prompt-take-back.ts:42 | Seed: draft storage full | — | | | | | | | |
| TST-094 | "Could not read the clipboard" (fallback daemon-unreachable text) | web/src/features/agent/composer/composer-plus-button.tsx:74 | Seed: clipboard read denied | — | | | | | | | |
| TST-095 | "Couldn't save this drawing" | web/src/features/agent/composer/excalidraw-takeover.tsx:67 | Seed: drawing upload refused | — | | | | | | | |
| TST-096 | "Could not attach that file"; "Could not attach that image"; "Could not attach that text" | web/src/features/agent/composer/lib/use-attachment-upload.ts:38 | Seed: attachment upload refused | — | | | | | | | |
| TST-097 | "Couldn't send that answer" | web/src/features/agent/hooks/use-answer-terminal-wait.ts:11 | Seed: terminal wait answer refused | — | | | | | | | |
| TST-098 | "Couldn't kill the agent on <chat>" | web/src/features/agent/lib/kill-agent.ts:26 | Seed: kill refused | — | | | | | | | |
| TST-099 | "Couldn’t send the prompt"; "Couldn’t take the prompt back"; "Couldn’t stop the turn" | web/src/features/agent/stores/chat-outbox.ts:173 | Seed: send/retract/stop refused | — | | | | | | | |
| TST-100 | "Move" (transcript move marker) | web/src/features/agent/transcript/rows/move-marker.tsx:61 | Seed: move marker action refused | — | | | | | | | |
| TST-101 | Browser: "Go back"; "Go forward"; "Reload the page"; "Go to the page"; "Hide the page"; "Show the page"; "Resize the page"; "Open the page"; "Cancel the pick"; "Could not pick"; "Close the browser tab" | web/src/features/browser/components/browser-pane.tsx:75 | Seed: native webview command failure | — | | | | | | | |
| TST-102 | "Language server action failed" | web/src/features/editor/components/toolbar/lsp-status-menu.tsx:72 | LSP chip > Restart. Seed: restart refused | — | | | | | | | |
| TST-103 | "Keep the unsaved draft"; "Discard the saved draft" | web/src/features/editor/documents/draft-store.ts:29 | Seed: IndexedDB write failure | — | | | | | | | |
| TST-104 | "Editor failed to load"; "Load the editor" | web/src/features/editor/host/use-editor-host.ts:12 | Seed: Monaco chunk failure | — | | | | | | | |
| TST-105 | "Failed to save file" | web/src/features/editor/lib/buffer-save.ts:68 | Seed: write refused | — | | | | | | | |
| TST-106 | LSP: "Format the document"; "Sync the file with its language server"; "Command failed"; "Could not open file"; "Rename failed"; "Formatting failed" | web/src/features/editor/lsp/monaco-lsp-providers.ts:231 | Seed: language server installed and failing | — | | | | | | | |
| TST-107 | "Create <.env target>" | web/src/features/file-explorer/file-explorer/hooks/use-env-template-file.tsx:84 | Seed: env template write refused | — | | | | | | | |
| TST-108 | "Copy the file content" | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:71 | Seed: file read refused | — | | | | | | | |
| TST-109 | "Rename failed"; "Paste failed" (file tree) | web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-inline-editing.ts:124 | Seed: rename/paste refused | — | | | | | | | |
| TST-110 | "Refresh the file tree"; "Reveal in Finder failed"; "Duplicate failed" | web/src/features/files/lib/file-tree-handlers.ts:95 | Files > Reveal in Finder with no file manager in the rig (Linux) | — | | | | | | | |
| TST-111 | "git unstage failed"; "git <stage\|discard>[-hunk] failed" | web/src/features/git/api/git-status-api.ts:20 | Seed: git action refused | — | | | | | | | |
| TST-112 | "Load your Git identity"; "Load the commit’s files" | web/src/features/git/hooks/use-current-identity.ts:27 | Seed: git identity/commit read refused | — | | | | | | | |
| TST-113 | "Set the console shortcut"; "Listen for the console menu" (macOS menu bridge) | web/src/features/keymaps/hooks/use-console-keyboard.ts:17 | Seed: console menu command failure | — | | | | | | | |
| TST-114 | "Load the document" (tab retry) | web/src/features/panes/components/editor-pane.tsx:142 | Seed: tab whose document fails to load, then retry | — | | | | | | | |
| TST-115 | "Stop the closed terminal"; "Clear blame for the closed file"; "Reopen the closed tab" | web/src/features/panes/lib/buffer-release.ts:76 | Seed: close/reopen cleanup failure | — | | | | | | | |
| TST-116 | "Could not clear browsing data" (macOS) | web/src/features/settings/components/tabs/browser-settings.tsx:34 | Seed: macOS clear-data failure | — | | | | | | | |
| TST-117 | "Diagnostics export failed"; "Could not add agent" | web/src/features/settings/components/tabs/developer-settings.tsx:139 | Seed: diagnostics or agent upload failure | — | | | | | | | |
| TST-118 | "Check open files against disk"; "Follow the file tree"; "Load the file tree"; "Load the folder"; "Refresh git status"; "Read the file from disk" | web/src/features/workspace/stores/hooks/use-workspace-file-tree.ts:41 | Seed: file-tree/git reads refused | — | | | | | | | |

Skipped:
- `web/src/components/ui/toast.tsx:39` AnchoredToasts (anchored and tooltip-style toasts, upsert replay animations `animate-toast-*`): `anchoredToastManager` has no caller.
- OS notifications: none (no `Notification` API use, no Tauri notification plugin).
- `web/src/components/layout/mobile/mobile-shell.tsx:89` and `web/src/components/oobe/oobe-screen.tsx:366` viewports: phone shell and OOBE (unreachable with a seeded project).
- `web/src/features/window/hooks/use-native-sidebar.ts:381` "Can't start a new thread yet": phone native sidebar.
- `web/src/features/agent/chat/use-dock.ts:40`, `web/src/features/agent/composer/plate/chat-markdown-editor.tsx:294`, `web/src/features/agent/transcript/agent-transcript.tsx:137`, `web/src/features/browser/lib/observe-rect.ts:20`, `web/src/features/hosts/lib/host-failures.ts:26`: local functions named `report`, not toasts.
