# ui

GPUI design system of the Crowbar shell. Every visual value comes from `docs/plans/shell-spec/00-tokens*.md`
(colour, type, radii, shadows, motion, z-order, primitives). The sidebar, tab bar, layout and settings crates
build on this crate and add no styling of their own.

## Setup

```rust
let application = gpui::Application::new().with_assets(ui::Assets);
application.run(|cx| {
    ui::init(cx)?;                       // fonts, Theme, Motion, key bindings
    ui::Overlays::install(host, cx);     // pane host implements ui::OverlayHost (optional until web views exist)
    // later, live: ui::Theme::of(cx).with_mode(ThemeMode::Dark).install(cx);
});
```

`Assets` serves the embedded icons; an app with its own assets wraps it and falls back to it.
`Theme::install` repaints every window, so switching theme, mode or UI font size is live. To follow the OS
appearance call `Theme::with_system(Appearance::from(window.appearance()))` from
`Window::observe_window_appearance`.

## Public API

Status: `ready` = implemented, unit tested, covered by the gallery.

| Area | Items | Status |
| --- | --- | --- |
| Theme | `Theme`, `ThemeId`, `ThemeMode`, `Appearance`, `Surface`, `Palette` (53 colour tokens as accessors returning `Hsla`), `contrast_ratio` | ready |
| Metrics | `radius::{SM,MD,LG,XL,XXL,FULL}`, `text_size::{XS..XL, UiText}`, `shadow::{xs,sm,md,lg,xs_tinted,dropdown,handle}`, `layer::*` (z order), `timing::*` (durations) | ready |
| Fonts | `font::{register, UI_FAMILY, HEADING_FAMILY, MONO_FAMILY}` (Cal Sans UI, Instrument Serif, Geist Mono) | ready |
| Icons | `Icon`, `IconName`, `IconWeight`, `Assets` (Phosphor: 56 icons, 92 weight files) | ready |
| Motion | `CubicBezier`, `Keyframes`, `Motion` (reduced motion, animation factory) | ready |
| Controls | `Button`, `ButtonVariant`, `ButtonSize`, `Switch`, `Checkbox`, `TextInput`, `Select`, `Tabs`, `TabItem`, `TabsStyle` | ready |
| Overlays | `OverlayHost`, `Overlays`, `OverlayGuard`, `Menu`, `MenuItem`, `MenuAction`, `DropdownMenu`, `Dialog`, `DialogSize`, `Tooltip`, `TooltipSide` | ready |
| Display | `Badge`, `Kbd`, `Toast`, `ToastKind`, `Separator`, `Skeleton`, `Spinner`, `EmptyState`, `ScrollArea`, `Sash`, `SashDrag`, `DragHandle`, `Axis` | ready |

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

`cargo run -p ui --bin gallery -- --theme crowbar --mode dark --section buttons --size 1200x800` renders every
component in every state. Sections: `buttons`, `inputs`, `toggles`, `feedback`, `structure`, `icons`,
`overlays`, `dialog`. The Docker recipe lives in `app/desktop/tests/docker`.

## Assets and licenses

`assets/fonts` and `assets/icons` are listed in `legal/NOTICE.md` with their licenses
(OFL-1.1 for the fonts, MIT for Phosphor).
