# Native phone Settings

Goal: the phone's Settings sheet navigates like a native iOS app (inset-grouped lists, push and
pop with swipe-back, native title and close) while keeping the app's own colours and keeping
the web's settings tabs where they are already built. Phone only; the desktop dialog is untouched.

## Today
A native sheet (`SettingsSheet.swift`) around one extra WKWebView at `#/mobile-settings`, which draws
the desktop tabs behind a title dropdown. Hosts and Agents are fed to it as snapshots
(`native-hosts-settings-feed.tsx`, `native-agents-settings-feed.tsx`); the page has no IPC.

## Structure
- Sheet: large detent, glass X top-left, centred inline title "Settings". Content is a SwiftUI
  `NavigationStack` (native push, swipe-back, back chevron).
- Root list, colours from the web palette (surface behind, raised cards):
  - Hosts, in one place only: the shown host (status dot, name, endpoint) is the row that pushes the
    native host list (select, swipe Revoke, Add host as its last row); with no host it reads
    "Add a host" and pushes the same list.
  - Appearance: three preview cards Light / Dark / System, the chosen one outlined.
  - "Theme" row (trailing value): pushes a native list of the registry's themes with a checkmark.
  - Page rows, each with an SF Symbol and chevron: Agents, Terminal, Git, Fonts & display.
- No log-out, account, (i), or haptics rows: the app has none of those settings.

## Native vs hosted React
- Native: root, host list, appearance cards, theme list. They are small, have no web-only widgets,
  and benefit most from native navigation. Their data is already main-webview state.
- Hosted: Agents, Terminal, Git, Fonts & display. These are the existing web tabs (sortable
  providers, selects, number inputs); rewriting them natively is out of scope. A pushed native
  screen holds the one warm settings WKWebView and tells it which tab to draw. Chosen over pushing a
  main-webview route because the main webview sits under the sheet and must keep its chat.

## Protocol additions
- `set_chrome.settings` grows to `{ open, theme, themes: [{id,name}], pages: [{id,label,symbol}] }`;
  `appearance` (theme mode) already rides along. The web owns the page inventory.
- Intents: `setThemeMode { mode }`, `setTheme { id }`, handled in `use-mobile-native-chrome.ts`.
  Host intents are the existing `selectHost`, `revokeHost`, `addHost`.
- Hosts: Swift decodes the raw `set_hosts_settings` snapshot it already receives (no second feed);
  the web Hosts page, its `hosts` channel and `MobileHostsSettings` are deleted.
- New page channel `tab` (`__crowbarSettingsTab`, ready `settingsTabReady`): Swift renders the
  page id string (or null on pop); `MobileSettingsPage` draws just that tab, with no header.

## Fonts
Native text uses the web's own faces, converted to TrueType (CoreText cannot read woff2) in
`desktop/ios-chrome/assets/`: `CalSansUI.ttf` (`--font-sans`, body rows, wght 400-700) and
`InstrumentSerif.ttf` (`--font-heading`, from @fontsource, section headers and nav titles). The Rust
half embeds both (`crowbar_chrome_font`) because a Swift package's resources do not reach the app;
`CrowbarFont.swift` registers them once. `crowbarUI` / `crowbarHeading` scale with Dynamic Type.
Alerts stay system.

## Rows
Every host and settings row shares `RowMetrics` (24pt icon column, 12pt gap, 20pt trailing column).
A host's icon is the web's `HostStatusIcon`: cloud for a remote, computer for `local`, status dot at
its corner. Cards are the list's stock background: a custom row background slides with a swiped row.

## Colours
`readNativePalette()` (`--background`, `--muted`, `--foreground`, `--muted-foreground`) is re-sent on
every theme change, so the sheet follows Crowbar/Frosted and Light/Dark with the web's pane colours.
Hosted pages read the same CSS tokens; the sheet takes the page's theme-color as before.

## Out of scope
Account/plan rows, promo row, info button, Editor/Files/Browser/Keybindings tabs (not on the phone),
per-theme preview swatches (themes are a list), a native rewrite of the hosted tabs.

## Hosted tabs on the phone
Phone only (`IS_MOBILE`); the shared `Section` / `SettingRow` / `SettingsCard` in
`settings-section.tsx` branch, so every tab inherits it. Section titles are the heading face at 17pt,
muted, above an inset card (rows divided inside one rounded surface). A row is label + 12pt muted
description on the left, the control on the right (max 55%); rule: at most ~2 short options or a
compact control inline, else `layout="stacked"` (same row chrome, control under the label).
Inline: switch, select (trigger sized to its value), stepper. Stacked: font pickers, long text fields,
the agents list (a card of its own rows). Rows set `data-setting-layout`; desktop sets none.

## Icons

The phone draws the desktop's Phosphor icons, never SF Symbols. `desktop/ios-chrome/scripts/gen-icons.mjs` reads the glyphs listed in `icons.json` from `@phosphor-icons/react` and writes `CrowbarIconData.swift` (path data as code; package resources do not reach the app). `PhosphorIcon` draws one in SwiftUI; `CrowbarIcon.image(size:)` renders a template `UIImage` for menus and swipe actions. Web payloads carry Phosphor names (`icon`), not symbol names. List rows push by hand so the caret is Phosphor's CaretRight. `make -C desktop lint` fails when the generated file is stale.
