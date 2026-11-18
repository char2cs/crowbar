# ui

GPUI design system of the Crowbar shell. Every visual value comes from `docs/plans/shell-spec/00-tokens*.md`
(colour, type, radii, shadows, motion, z-order, primitives). The sidebar, tab bar, layout and settings crates
build on this crate and add no styling of their own.

## Setup

```rust
let application = gpui::Application::new().with_assets(ui::assets());
application.run(|cx| {
    ui::init(cx)?;                       // fonts, Theme, Motion, key bindings
    ui::Overlays::install(host, cx);     // pane host implements ui::OverlayHost (optional until web views exist)
    // later, live: ui::Theme::of(cx).with_mode(ThemeMode::Dark).install(cx);
});
```

`ui::assets()` serves the embedded icons; an app with its own assets wraps it and falls back to it.
`Theme::install` repaints every window, so switching theme, mode or UI font size is live. To follow the OS
appearance call `Theme::with_system(Appearance::from(window.appearance()))` from
`Window::observe_window_appearance`.

## Public API

Status: `verified` = implemented, unit tested and rendered in the gallery (crowbar light/dark); `unverified` = implemented and unit tested but not yet compared against the React original; `missing` = not implemented.

| Area | Items | Status |
| --- | --- | --- |
| Theme | `Theme`, `ThemeId`, `ThemeMode`, `Appearance`, `Surface`, `Palette` (53 colour tokens as accessors returning `Hsla`), `contrast_ratio` | verified |
| Metrics | `radius::{SM,MD,LG,XL,XXL,FULL}`, `text_size::{XS..XL, UiText}`, `shadow::{xs,sm,md,lg,xs_tinted,dropdown,handle}`, `layer::*` (z order), `timing::*` (durations) | unverified |
| Fonts | `font::{register, UI_FAMILY, HEADING_FAMILY, MONO_FAMILY}` (Cal Sans UI, Instrument Serif, Geist Mono) | verified |
| Icons | `Icon`, `IconName`, `IconWeight`, `assets()` (Phosphor: 56 icons, 92 weight files) | verified |
| Motion | `CubicBezier`, `Keyframes`, `Motion` (reduced motion, animation factory) | verified |
| Controls | `Button` (with `loading()`), `ButtonVariant`, `ButtonSize`, `Switch`, `Checkbox`, `TextInput`, `Select`, `Tabs`, `TabItem`, `TabsStyle` | verified |
| Text area | `TextArea`, `TextAreaSize`, `TextAreaCondition` | unverified (no React golden at component level) |
| Overlays | `OverlayHost`, `Overlays`, `OverlayGuard`, `Menu`, `MenuItem`, `MenuAction`, `DropdownMenu`, `ContextMenu` (right click, `MenuAction::submenu`, arrow/left/right keys), `Popover`, `Dialog`, `DialogSize`, `Tooltip`, `TooltipSide` | unverified |
| Display | `Badge`, `Kbd`, `Toast`, `ToastKind`, `ToastQueue`, `ToastContent`, `ToastId`, `ToastLifetime`, `ToastPlacement`, `ToastSide`, `ToastPhase`, `ToastQueueEvent`, `Separator`, `Skeleton`, `Spinner`, `ProgressBar`, `EmptyState`, `ScrollArea`, `Sash`, `SashDrag`, `DragHandle`, `Axis` | verified |

## Conventions for consumers

- Every component takes `.on(Surface::Chrome)` when it sits in the sidebar or the view-tabs row (crowbar keeps
  them dark in light mode). Popups, dialogs and tooltips always use `Surface::Window`.
- Colours: `Theme::of(cx).palette(Surface::Window).border()` and friends. Never hard-code a hex value.
- Hover and focus: components fade colours over 150ms and draw the focus ring only for keyboard focus.
- Overlays tell the shell which window rectangle they cover through `OverlayHost`; the pane host hides web
  views under it. A modal dialog reports the whole window.
- Reduced motion: install `Motion::reduced()` as a global when the OS asks for it; every animation collapses
  to 10 microseconds, spinners keep turning.

## Gallery

`cargo run -p ui --bin uigallery -- --theme crowbar --mode dark --section buttons --size 1200x800` renders every
component in every state. Sections: `buttons`, `inputs`, `toggles`, `feedback`, `structure`, `icons`,
`overlays`, `dialog`, `textarea`, `toasts`. The Docker recipe lives in `app/desktop/tests/docker`.

## Settings modal additions

`TextInputStyle` (`Standard`, `Compact` 28px, `Stepper` 24px centred) with `TextInput::style` and `leading_icon`;
`hover_fill` (150ms background fade on hover); `DialogExtent` with `Dialog::flush` (fixed-size dialog with an
unruled header and no body padding); `Select::compact` (28px trigger); `IconName::Minus`.

## Text area

`TextArea` is a multi-line `TextInput`: it shares `TextBuffer` (selection, graphemes, UTF-16 for IME), the clipboard
commands and `TextInputEvent` with the single-line field. Build it with `TextArea::build(placeholder, cx)` (or
`cx.new(|cx| TextArea::new(..).size(..).max_height(..))`), render the entity, subscribe to `TextInputEvent`.

- `Enter` inserts a newline, `Ctrl+Enter` (`Cmd+Enter`) emits `Submitted`, `Escape` emits `Cancelled`, like the React
  `onKeyDown` of the review composer and commit form.
- Wraps to its width, grows with its text from three rows (`min-h-17.5`), stops at `max_height` and scrolls; the
  caret is kept in view, the wheel scrolls.
- Up/Down keep the column across rows, Home/End go to the visual row edge, `Ctrl+Home/End` to the document edges,
  `Ctrl+Z`, `Ctrl+Shift+Z`/`Ctrl+Y` undo and redo (typed runs undo as one step, an IME composition as one step).
- `TextAreaSize` (`Small` 66, `Default` 70, `Large` 74 minimum box), `invalid()` (destructive border, ring at
  16%/24%), `disabled()` (64% opacity, no focus, no editing); `set_condition` switches them at runtime.
- Double click selects a word, triple click the line; `Ctrl`/`Alt` + arrows, `Backspace` and `Delete` move, select
  and delete by word; Page Up/Down (text area) move a viewport. Both fields keep selecting while the pointer is
  held outside them, and the text area scrolls a row every 50ms while the pointer is above or below it.

## Toasts

`Toast` stays a stateless card. New: `on_close(handler)` (corner close button, 14px X, 50% opacity, 100% and a muted
fill on hover), `full_width()`, `leaving()` (fade out), a 200ms fade in on first paint, and the success pulse.
GPUI has no per-element scale, so the pulse widens and narrows the card box with a negative percentage margin
(2.5% at the 30% keyframe, 0.99 at 60%); the text inside is not scaled.

`ToastQueue` is the entity that owns the list, like the base-ui manager behind `SidebarToastOverlay`: newest first,
`push(ToastContent, ToastLifetime, cx)`, `replace(id, content, cx)` (replays shake or pulse, restarts expiry),
`dismiss(id, cx)` (200ms fade, then removed and `ToastQueueEvent::Removed`). `ToastLifetime::Auto` expires after five
seconds, `Pinned` never (the `timeout: 0` toast); loading toasts never expire either. `ToastPlacement::Docked(side)`
is the open-sidebar column (full width, newest at the bottom, three transient toasts, pinned ones always kept),
`Floating(side)` the collapsed-sidebar column (288px, 16px from the bottom and the side, every toast). Call
`set_placement` when the sidebar collapses. `show(key, (content, lifetime), cx)` rewrites the live toast of a key
in place and `clear(key, cx)` dismisses it, so owners keep no second list of ids. Swiping a card 40px toward the sidebar edge (or down) dismisses it; the
gesture only tracks while the pointer is over the card.

Mount the queue inside a relatively positioned container: it positions itself absolutely.

## Skeleton

The highlight now follows the CSS original: a 120 degree stripe fixed to the window (`background-attachment: fixed`,
2x viewport tile), starting one second into the cycle, in phase for every skeleton through `ShimmerClock`
(installed by `ui::init`; without it each frame starts from zero).

## Assets and licenses

`assets/fonts` and `assets/icons` are listed in `legal/NOTICE.md` with their licenses
(OFL-1.1 for the fonts, MIT for Phosphor).


Missing: `ui::root()` (every consumer sets the window background itself; no shared base wrapper earned its keep), emoji font bundling (emoji render through the system colour-emoji font that GPUI's text system falls back to; hosts without Noto Color Emoji show tofu), a true italic UI face (see below).

Italics: Cal Sans UI has no italic face and GPUI does not synthesise an oblique, so `.italic()` (provisional titles) renders upright. Fix path: bake a 0.25-shear face (glyf and gvar deltas) with fontTools and register it as a second face of `Cal Sans UI`; not done because fontTools was unavailable offline. Pixel comparison against the React originals is not done: the golden captures are whole-app screens, not component captures. The frosted theme and Tooltip hover motion have not been looked at in the gallery.
