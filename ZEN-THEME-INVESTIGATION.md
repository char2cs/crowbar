# Zen Theme Match — Investigation Log

## Goal
Make Crowbar's **"Crowbar" theme** (dark, macOS) visually match **Zen Browser's
default look** for two surfaces: the **content pane background** and the
**sidebar background** — same colors, same opacity, same feel. User is on macOS;
both apps render over native window vibrancy and the same desktop wallpaper.

## Hard constraints / facts about the setup
- **Crowbar**: Tauri app. Transparent window + native macOS vibrancy applied via
  the `window-vibrancy` crate in `desktop/src-tauri/src/lib.rs` (~line 255).
  Theme tokens live in `web/src/styles/theme.css` (oklch), hot-reloaded by Vite
  (`tauri dev` running from THIS worktree; webview debug port 9223).
- **Zen**: Firefox fork cloned at `~/Projects/Cloned/desktop`. Native vibrancy via
  `widget/cocoa/VibrancyManager.mm` (Zen patch at
  `src/widget/cocoa/VibrancyManager-mm.patch`). Content/sidebar background colors
  computed by `src/zen/spaces/ZenGradientGenerator.mjs`; theme vars in
  `src/zen/common/styles/zen-theme.css`, `zen-browser-ui.css`,
  `zen-browser-container.css`.
- **The assistant CANNOT take native screenshots** (`screencapture` → "could not
  create image from display"; no screen-recording permission). The Tauri MCP
  `webview_screenshot` (CDP) captures only the web content composited over a flat
  default backdrop — it does **not** show the native vibrancy. **All visual
  verification depends on user-supplied screenshots.** This is the core reason
  iteration is slow.

## Zen's verified configuration (from CODE + the user's live profile)
Profile: `~/Library/Application Support/zen/Profiles/3y8atrxz.Default (release)/`
- `zen-themes.json` is `{}` (empty) and no `gradientColors` in any DB → user is on
  the **DEFAULT theme** (no custom workspace gradient).
- No `zen.widget.macos.window-material` override → default value **`1`** →
  `NSVisualEffectMaterialHUDWindow` (per the VibrancyManager patch's switch).
- No `zen.view.grey-out-inactive-windows` override → default **`true`** →
  vibrancy state = `NSVisualEffectStateFollowsWindowActiveState`.
- No `zen.theme.acrylic-elements` override → default **`false`** →
  `allowTransparencyOnSidebar = false`.

Resulting Zen values:
- **Material:** HUDWindow. **State:** FollowsWindowActiveState. **Blend:** BehindWindow.
- **Content pane bg (dark, macOS, default theme):** `getGradient()` →
  `getBrowserBg()` → **`rgba(0,0,0,0.4)`**. (Light → `transparent`.)
- **Sidebar** `#zen-main-app-wrapper` bg = `--zen-themed-toolbar-bg-transparent`
  = **`transparent`** on macOS (pure vibrancy). The tab/toolbar strip paints
  `--zen-main-browser-background-toolbar` = `getToolbarModifiedBase()` =
  `rgba(23,23,26, 1)` (acrylic off) = solid `#17171a` — but only over the
  toolbar/tab region, not the empty sidebar.
- **Content card:** rounded (`--zen-native-inner-radius`) + `box-shadow:
  var(--zen-big-shadow)` = `rgba(0,0,0,0.24) 0px 3px 8px` (subtle).

## Changes currently applied to Crowbar
`web/src/styles/theme.css`:
- `.dark --pane-background`: `oklch(0 0 0 / 40%)`   (was `oklch(0.19 0 0 / .75)`) — = Zen `rgba(0,0,0,0.4)`.
- `.dark --sidebar`: `oklch(0 0 0 / 18%)`           (was `oklch(0.18 0 0 / 70%)`) — tint, user asked for body.
- `.dark --chrome-bg`: `oklch(0 0 0 / 0%)`          (was `oklch(0 0 0 / 50%)`) — removed window-wide film.
- `:root` (light) `--pane-background`: `oklch(0 0 0 / 0%)`; `--sidebar`: `oklch(0 0 0 / 0%)`.

`desktop/src-tauri/src/lib.rs` (~line 260):
- `NSVisualEffectMaterial::HudWindow` (matches Zen; reverted after a wrong detour to `Sidebar`).

## Journey / missteps (do NOT repeat)
1. Swapped material HudWindow→Sidebar believing Sidebar was Zen's and lighter. **Wrong** — Zen uses HudWindow. Reverted.
2. Content overlay churned: black@40% → white@12% veil → warm taupe `#857a6c`@40% → back to black@40%. The detours came from **reverse-engineering rendered pixels**, which were confounded because the Zen window sat over a brighter wallpaper patch than Crowbar. Code+profile prove the real value is **`rgba(0,0,0,0.4)`**.
3. Sidebar: transparent (too see-through per user) → added `black@18%` tint.

## THE OPEN PROBLEM ("something big is still missing")
Even with chrome compositing matched to Zen's verified config, Crowbar looks clearly different:
- A **stark dark rounded-rectangle ("the blob")** dominates the content area, with
  noticeably **brighter margins** around it. **Critically: the blob stays in the
  same position relative to the window and does NOT move when the window moves.**
  → It is therefore **drawn by the app**, not the wallpaper behind the glass.
- Zen's content area reads as **even, warm, frosted glass**; Crowbar reads as a
  **dark card floating on bright margins** (high local contrast).
- The CDP/webview-only screenshot showed the content area flat gray with no blob,
  but CDP flattens translucency over its own backdrop, so that does **not** prove
  the blob is non-DOM.

### Leading hypothesis
The blob = Crowbar's **pane-container** (`features/panes/components/pane-container.tsx`,
`bg-pane-background` = black@40%, rounded corners, **margins**, and possibly a
**heavy drop-shadow** — `web/src/index.css` has special handling for the pane
shadow "bleeding" into the sidebar). The dark-card-on-bright-margin contrast is
what Zen does not have. Needs definitive confirmation + the right structural fix.

## Key measurements (region averages; L = approx oklch lightness)
Controlled-ish pair, both over same backdrop (img 23 Crowbar / 24 Zen):
- Content frost: Crowbar `#2f2820` L0.28  vs  Zen `#64594b` L0.47.
- Sidebar: Crowbar `#3f342a` L0.33  vs  Zen `#382b1d` L0.30.
Bare Crowbar vibrancy estimate (content) ≈ `#4e4335`.

## Key files
- Crowbar: `web/src/styles/theme.css`, `web/src/components/layout/ide-shell.tsx`,
  `web/src/features/panes/components/pane-container.tsx`, `web/src/index.css`,
  `desktop/src-tauri/src/lib.rs`, `desktop/src-tauri/tauri.conf.json`.
- Zen: `src/zen/common/styles/{zen-theme,zen-browser-ui,zen-browser-container}.css`,
  `src/zen/spaces/ZenGradientGenerator.mjs`, `src/widget/cocoa/VibrancyManager-mm.patch`.

## Reference screenshots (image cache)
`/Users/char2cs/.claude/image-cache/2cc84c13-8419-4a8d-ad27-8100df61950e/`
- `29.png` = Crowbar CURRENT. `28.png` = Zen reference.
- `23.png` / `24.png` = controlled Crowbar/Zen pair over same backdrop.

---

# PART 2 — Bottom-up review + DETERMINISTIC direction (why CSS-guessing failed)

## What actually happened (review)
- Dark theme: SOLVED and confirmed by user. Mechanism: full-window dark wash
  (`--chrome-bg: oklch(0 0 0/40%)` on <body>) + content frost (`--pane-background:
  oklch(1 0 0/10%)`) + HudWindow vibrancy. Works because the OS is in DARK mode →
  the native NSVisualEffectView frost renders DARK → aligns with the dark wash.
- Light theme: NOT solved after ~10 CSS-value guesses. Root issue identified:

## THE deterministic root cause (proven from code, not pixels)
1. `window-vibrancy` 0.6.0 `apply_vibrancy` (registry .../internal.rs) sets
   material/blendingMode/state but **never sets the view's appearance** → the
   NSVisualEffectView inherits `effectiveAppearance`.
2. `effectiveAppearance` follows NSApp/window/system. The OS is DARK → the frost
   is DARK in BOTH themes.
3. Dark theme wants a dark frost → perfect. Light theme wants a LIGHT frost but
   gets a dark one → the content (over a dark desktop region) reads gray, and a
   window-wide CREAM wash cannot lift a dark frost cleanly → endless tuning.
4. Tauri `setTheme` sets **NSApplication.appearance** (tao set_ns_theme:
   `msg_send![app, setAppearance:]`). The user's `setTheme('dark')` test DID
   darken the window → appearance DOES drive the frost. So the lever is real;
   the bridge `setMacOSWindowAppearance` was a **no-op stub** (now implemented to
   call setTheme), but app-level appearance is fragile/inconsistent.

## Why "copy Zen" is the answer
Zen (Gecko) sets the native window appearance to match the browser color-scheme,
so in light theme the SAME HudWindow material renders as a LIGHT frost. That is
the ONLY reason Zen's light content is light. We must replicate that exact
native mechanism, then use Zen's EXACT per-surface CSS values — not guessed CSS.

## Deterministic plan (to implement, no guessing)
A. NATIVE: set the vibrancy NSVisualEffectView's (or the NSWindow's) appearance
   to Aqua (light) / DarkAqua (dark) per theme, in Rust — reliably, not via the
   app-level setTheme. This makes the frost render per-theme by construction
   (documented macOS behavior).
B. CSS: use Zen's EXACT per-surface overlays (default theme, macOS):
   - content `--zen-main-browser-background`: dark `rgba(0,0,0,0.4)`, light `transparent`.
   - sidebar `#zen-main-app-wrapper`: `transparent` (both).
   - toolbar/tabstrip `--zen-main-browser-background-toolbar` = getToolbarModifiedBase.
   Apply each to the matching Crowbar element (NOT a single window-wide wash that
   over-lightens the light content). Keep the WORKING dark theme intact.
C. Verify: cargo build OK, app runs, DOM computed values correct, native command
   returns OK. Logic is deterministic (replicating Zen), so correct-by-construction.

## Constraints
- Assistant cannot take NATIVE screenshots (no screen-rec perm; webview/CDP shots
  don't show vibrancy). Verification of the FRAME is the user's; the IMPLEMENTATION
  must be correct by construction (Zen-exact), not pixel-tuned.
- Dark theme MUST keep working (user approved it).

---

# PART 3 — DETERMINISTIC SOLUTION (implemented + verified)

## Root cause (final, proven)
The vibrancy frost follows the NSVisualEffectView's `effectiveAppearance`.
window-vibrancy never sets it → it inherits the OS (dark) in BOTH themes. Dark
theme wants a dark frost (perfect); light wants a LIGHT frost but got a dark one.
Tauri `setTheme` only flips app-level NSApp.appearance (fragile, inconsistent).
Zen/Gecko fixes this by mapping the chrome color-scheme → the native window
appearance. The deterministic equivalent: pin the blur VIEW's appearance directly.

## The fix (3 files)
1. desktop/src-tauri/src/lib.rs — new `#[tauri::command] set_vibrancy_appearance(window, dark)`:
   finds the window-vibrancy blur view by tag (NS_VIEW_TAG_BLUR_VIEW = 91376254)
   and sets its NSAppearance to Aqua (light) / DarkAqua (dark) + setNeedsDisplay
   (NSWindow fallback; never NSApp). Registered in generate_handler!.
2. web/src/lib/crowbar-bridge.ts — `setMacOSWindowAppearance` now invokes that
   command (was a no-op stub, then a fragile setTheme).
3. web/src/styles/theme.css (:root / light ONLY):
   --pane-background: oklch(0 0 0 / 0%)   (Zen getBrowserBg() light = transparent)
   --chrome-bg:       oklch(0 0 0 / 0%)   (Zen #main-window transparent on macOS)
   Light is now 100% native (Aqua-pinned) vibrancy, exactly like Zen.

## DARK theme: byte-for-byte unchanged
--pane-background oklch(1 0 0/10%), --sidebar oklch(0 0 0/18%), --chrome-bg
oklch(0 0 0/40%); apply_vibrancy(HudWindow) unchanged. Dark calls the command
with dark=true → DarkAqua → identical to today's OS-dark frost.

## Verification (deterministic, done)
- `cargo check` → exit 0 (command compiles).
- `tauri dev` auto-rebuilt the binary (mtime post-edit) + relaunched (new PID).
- `set_vibrancy_appearance` invoked end-to-end → resolved **OK** (window.__vibResult).
- Bridge runs on startup (settings-bootstrap) + every theme switch → persists.
- crowbar-bridge unit tests: 10/10 pass.
- Live DOM (light): body bg = transparent, content pane = transparent (Zen-exact).
- Diff isolated to the 3 files above; dark tokens untouched.
- Remaining tsc errors are pre-existing duplicate-@types/react noise (not my files).

## Why this is deterministic (not guessing)
Pinning the blur view to Aqua makes the HUDWindow material render a LIGHT frost —
documented NSVisualEffectView behavior, the SAME mechanism Zen uses. No CSS
counter-wash, no opacity tuning. The only thing the assistant cannot self-check is
the final rendered pixels (no native screenshot access); the mechanism is correct
by construction.

## Optional follow-up (not done; only if user wants it)
Zen's light tab/toolbar strip is a solid #f0f0f4 band (getToolbarModifiedBase).
Crowbar's strip is currently pure vibrancy. Add a light-only --tab-strip-bg if the
user wants that exact band; dark stays transparent. Content + sidebar already match.

---

# PART 4 — Refinement: window-level appearance pin (matches Zen's frost lightness)

Measured Zen(45) vs Crowbar(46) light, after the first native fix:
  content  Zen #c9c8c8 L0.834  vs  Crowbar #777676 L0.567
  margin   Zen ~L0.92          vs  Crowbar ~L0.79
Crowbar's frost was uniformly DARKER. Cause: the command pinned the blur VIEW's
appearance *explicitly*, which renders darker than Zen's WINDOW-level pin.

Fix (lib.rs set_vibrancy_appearance): pin the NSWINDOW appearance and CLEAR the
blur view's own appearance (nil) so it INHERITS the window — exactly Zen/Gecko's
mechanism (color-scheme → NSWindow.appearance, view inherits). Safe: CSS theming
is `.dark`-class based, not prefers-color-scheme; only the JS *system* theme-mode
detector reads prefers-color-scheme (user uses explicit light/dark). Verified:
cargo check 0, relaunched, command resolves OK, light CSS still transparent, dark
untouched.

NOTE on comparison: screenshots 45/46 were different window SIZES (2000x1596 vs
2000x1520) → different wallpaper behind → part of the measured gap is position.
Compare both windows at the SAME size/position for a true read.

---

# PART 5 — Final: content tint + system-mode fix

## The actual content requirement (what the user meant all along)
The CONTENT PANE (where terminals/editors open) must carry Zen's content-webview
fill, NOT be transparent. Zen paints the webview itself in
zen-browser-container.css:21 — browser[type=content][transparent=true]:
  light = rgba(255,255,255,0.6)   dark = rgba(255,255,255,0.1)
I had been reading getBrowserBg() (the chrome BEHIND the webview = transparent),
the wrong layer. Fix: theme.css :root --pane-background = oklch(1 0 0 / 60%)
(white@60%, Zen's light value). Dark already = white@10% (Zen's dark value) —
that symmetry is why dark always worked. Confirmed: white@60% over the measured
content vibrancy #787775 = #c9c8c8 = Zen's content. User: "that nailed it."

## System-mode regression + fix
The window-level appearance pin (PART 4) also flipped the WKWebView's
effectiveAppearance → prefers-color-scheme, which the JS `system` theme mode
reads to follow macOS → "doesn't react to light/dark." Fix: pin ONLY the blur
view (frost), never the NSWindow/webview. Verified: OS=light AND webview
prefers-color-scheme=light (matching) → system-mode following restored; command
resolves OK; content tint preserved.

## FINAL state (all in code)
- lib.rs set_vibrancy_appearance: pins the blur VIEW appearance (Aqua/DarkAqua)
  per theme; never touches NSWindow/NSApp. Run on startup + every theme switch.
- crowbar-bridge.ts setMacOSWindowAppearance → invokes it.
- theme.css: light --pane-background white@60% (Zen content), dark white@10%
  (Zen content); light --chrome-bg 0% (no wash); sidebar transparent; dark block
  untouched.
