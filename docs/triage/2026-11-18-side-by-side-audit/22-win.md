# 22. Window chrome (WIN)

| ID | What | Tauri source | How to reach/drive it | Animation | Shots | Tauri behaviour | Rust behaviour | Difference | Perf | Linux | macOS |
|---|---|---|---|---|---|---|---|---|---|---|---|
| WIN-001 | Linux window at launch: 1200×800, resizable, opaque, native GTK decorations (title bar "Crowbar" with minimise/maximise/close) | desktop/src-tauri/tauri.linux.conf.json:4 | rig start tauri; rig size standard | — | | | | | | | |
| WIN-002 | macOS window at launch: 1200×800, transparent, title bar Overlay with hidden title, traffic lights at (12, 23) | desktop/src-tauri/tauri.conf.json:23 | macOS: launch | — | | | | | | | |
| WIN-003 | macOS vibrancy: HudWindow blur behind the transparent window, state Active (blur kept while unfocused) | desktop/src-tauri/src/window_chrome.rs:9 | macOS: focus another app; compare | — | | | | | | | |
| WIN-004 | macOS vibrancy appearance pinned to the app theme (light frost in light, dark in dark), independent of OS appearance | desktop/src-tauri/src/window_chrome.rs:36 | macOS: rig theme light, rig theme dark | — | | | | | | | |
| WIN-005 | Window title: "Crowbar"; with a chat pane focused "<chat title> — Crowbar" | web/src/features/window/hooks/use-document-title.ts:14 | click the Perf fixture chat tab; then a non-chat pane; read GTK title bar | — | | | | | | | |
| WIN-006 | No minimum window size: content keeps shrinking with the window | desktop/src-tauri/tauri.conf.json:23 | rig size min; rig size 400x300 | — | | | | | | | |
| WIN-007 | No window state restore: size, position and maximised state not persisted; every launch is 1200×800 | desktop/src-tauri/src/lib.rs:56 | rig size 900x600; rig restart tauri | — | | | | | | | |
| WIN-008 | Linux maximise / restore via the GTK title bar (button or double-click) | desktop/src-tauri/tauri.linux.conf.json:9 | double-click the GTK title bar twice | WM-driven | | | | | | | |
| WIN-009 | Drag region: sidebar header row (Settings, host picker gap, sidebar toggle) moves the window | web/src/components/layout/sidebar-project-header.tsx:77 | drag an empty spot in the sidebar header | — | | | | | | | |
| WIN-010 | Drag region: pane top row (tab bar/chat header shell) | web/src/features/tabs/components/pane-top-row.tsx:61 | drag empty space in a pane's top row | — | | | | | | | |
| WIN-011 | Drag region: view-tabs row | web/src/features/tabs/components/view-tabs-row.tsx:83 | drag empty space right of the view tabs | — | | | | | | | |
| WIN-012 | Drag region: tab bar trailing empty area | web/src/features/tabs/components/tab-bar.tsx:384 | drag empty space after the last tab | — | | | | | | | |
| WIN-013 | Drag region: console header | web/src/features/console/components/console-header.tsx:33 | open the console (see PANE); drag its header | — | | | | | | | |
| WIN-014 | Drag region: sidebar nav-stack header | web/src/components/layout/nav-stack.tsx:48 | open a sidebar nav screen (see SB); drag its header | — | | | | | | | |
| WIN-015 | Double-click on a drag region toggles maximise (Tauri drag-region behaviour) | web/src/components/layout/sidebar-project-header.tsx:77 | double-click empty sidebar header | WM-driven | | | | | | | |
| WIN-016 | macOS traffic-light clearance: 72px spacer at the sidebar header's left when the sidebar is on the left | web/src/components/layout/sidebar-project-header.tsx:81 | macOS: rig sidebar left | — | | | | | | | |
| WIN-017 | macOS traffic-light clearance: pane top row at the window's top-left gets 88px left padding (sidebar hidden or right) | web/src/features/tabs/components/pane-top-row.tsx:57 | macOS: hide sidebar; rig sidebar right | — | | | | | | | |
| WIN-018 | macOS traffic-light clearance: console header pl-20 when docked top or left; nav-stack 72px spacer | web/src/features/console/components/console-header.tsx:37 | macOS: open console docked top | — | | | | | | | |
| WIN-019 | macOS traffic lights follow the sidebar side: with the sidebar right they move to the top-left pane row (+12, +23); re-applied on resize, theme switch and OS scheme change | web/src/features/tabs/hooks/use-mac-traffic-light-sync.ts:47 | macOS: rig sidebar right; rig theme light; resize | applied two frames after a theme switch | | | | | | | |
| WIN-020 | Linux has no traffic-light inset anywhere (all spacers macOS-only) | web/src/components/layout/sidebar-project-header.tsx:81 | rig sidebar left; observe header starts with Settings | — | | | | | | | |
| WIN-021 | Chrome row height 42px (32px tab + 2×5px) shared by pane headers, view-tab strip, sidebar header | web/src/lib/chrome-metrics.ts:6 | measure sidebar header vs pane top row | — | | | | | | | |
| WIN-022 | Linux chrome surfaces opaque: --chrome-bg/--pane-chrome-bg flattened (light oklch .989/.986; dark oklch .182 .005 106.7) | web/src/styles/theme.css:310 | rig theme light; rig theme dark | — | | | | | | | |
| WIN-023 | Crowbar theme light mode on Linux: chrome dark opaque oklch(.182 .005 106.7), pane chrome = pane background | web/src/styles/crowbar.css:40 | rig theme light with theme Crowbar | — | | | | | | | |
| WIN-024 | Crowbar theme light mode on macOS: translucent chrome (sidebar black/18%, chrome 65% ink mix) over vibrancy | web/src/styles/crowbar.css:16 | macOS: rig theme light | — | | | | | | | |
| WIN-025 | html carries data-platform (linux/macos), data-theme, data-theme-type and .dark from the bootstrap cache | web/src/features/settings/lib/appearance-bootstrap.ts:134 | inspect <html> after rig restart tauri | — | | | | | | | |
| WIN-026 | Window close (title bar ×): page receives pagehide first (drafts, held removals flushed), then the window is destroyed | web/src/lib/crowbar-bridge.ts:283 | trash a workspace, close the window within 8s; reopen (see WS-033) | — | | | | | | | |
| WIN-027 | Last window closed: daemon sidecar shut down with the app | desktop/src-tauri/src/windows.rs:106 | close the window; check the daemon process | — | | | | | | | |
| WIN-028 | macOS Cmd+Q: app exit shuts the daemon down (no per-window close events) | desktop/src-tauri/src/lib.rs:232 | macOS: Cmd+Q | — | | | | | | | |
| WIN-029 | macOS New Window (Cmd+Alt+N): second window cloned from the main config, opens at / (lands on active project), decorated with vibrancy | desktop/src-tauri/src/windows.rs:22 | macOS: Window > New Window (see MENU) | — | | | | | | | |
| WIN-030 | Second window shares hosts and settings with the first live (storage events) | web/src/features/hosts/stores/host-config-store.ts:78 | macOS: two windows; change theme in one | — | | | | | | | |
| WIN-031 | Closing a non-last window keeps the daemon up and only drops that window's bridge sockets | desktop/src-tauri/src/windows.rs:93 | macOS: open 2 windows, close one | — | | | | | | | |
| WIN-032 | Window blur: no inactive-window styling; an open sidebar peek closes and an in-progress sidebar resize/drag ends | web/src/components/layout/sidebar-peek.tsx:75 | show the sidebar peek, then focus another X window | — | | | | | | | |
| WIN-033 | Unfocused/occluded window keeps running at full rate (background throttling disabled) | desktop/src-tauri/tauri.conf.json:36 | minimise the window during a streaming update; restore | — | | | | | | | |
| WIN-034 | Chat zoom in / out / reset (mod+= / mod+- / mod+0 default): chat surface zoom 0.3–3.0 in 0.1 steps; reset also resets editor and terminal zoom levels; key repeat ignored | web/src/features/keymaps/hooks/use-zoom-keyboard.ts:21 | focus the chat; rig key tauri ctrl+equal ×3, ctrl+minus, ctrl+0 (see KEY) | — | | | | | | | |
| WIN-035 | Chat zoom not persisted: back to 100% after relaunch | web/src/features/window/stores/zoom-store.ts:25 | zoom in; rig restart tauri | — | | | | | | | |
| WIN-036 | No app-level fullscreen; macOS green traffic light gives native fullscreen | desktop/src-tauri/tauri.conf.json:23 | macOS: click green button | OS-driven | | | | | | | |
| WIN-037 | Webview never navigates away: web links open in the OS browser | desktop/src-tauri/src/lib.rs:61 | click an http link in chat markdown | — | | | | | | | |

Skipped:
- desktop/src-tauri/src/dev_identity.rs:66 dev title "Crowbar — dev: <label>" and dev Dock icon: debug builds with CROWBAR_DEV_LABEL only.
- No system tray: nothing on develop creates one.
- desktop/src-tauri/src/app_menu.rs: native app menu (incl. New Window / Close Window) is macOS-only (`#[cfg(target_os = "macos")]`), so Linux has a single window and no menu bar; menu items are MENU's.
- web/src/lib/native-chrome.ts: iOS native chrome; hasNativeChrome() is false on desktop.
- web/src/features/window/hooks/use-surface-theme-color.ts: theme-color meta feeds the phone's native chrome only.
- web/src/features/window/hooks/use-mobile-native-chrome.ts, use-native-sidebar.ts, use-native-tabs.ts, use-native-files.ts, mobile-*-page.tsx: phone only.
- desktop/src-tauri/src/frame_rate.rs, tauri-plugin-macos-fps: macOS 60fps→display-rate uncap, not visible as a state.
