//! `--surface native-menu` — a **real** `NSMenu`, and the one surface with no
//! anchored-geometry gate.
//!
//! Crowbar's dropdown menus are native rather than simulated. The popup this
//! surface opens is drawn by the operating system, not by GPUI, so there is no
//! element tree to record and nothing for the differ to compare: a snapshot of
//! this surface would describe the plate below the menu and say nothing about
//! the menu at all. That is the decision, not a gap in it — see
//! [`crowbar_platform::native_menu`] for what the platform gives back in
//! exchange.
//!
//! What the surface therefore is: a **trigger**, and the menu it opens.
//!
//! # How to judge it — one line each
//!
//! Every check below is done by eye, on a real menu, because that is what "the
//! OS draws it" means. Nothing here is asserted by a test, and pretending
//! otherwise would be inventing an oracle for a surface that deliberately has
//! none. Run `cargo run -p crowbar-app -- --surface native-menu` and:
//!
//! | # | Behaviour | How to check it, in one line |
//! |---|---|---|
//! | 1 | **It is the real thing** | Right-click the plate: the popup has the system's own translucency, corner radius and font, and does **not** repaint when you switch Crowbar between light and dark with `--theme`. |
//! | 2 | **Keyboard navigation** | With the menu open press ↓ ↓ ↑ — the highlight moves one row per press and skips the separator without stopping on it. |
//! | 3 | **Type-select** | Type `de` with the menu open: the highlight jumps to `Delete`, which no drawn menu in this port does. |
//! | 4 | **Escape** | Press Escape: the menu closes and the plate reads `dismissed with nothing chosen`. |
//! | 5 | **Click-outside** | Click anywhere outside the popup: same as 4, and the click is **swallowed** rather than reaching what is under it. |
//! | 6 | **Choosing a row** | Click `Copy as Markdown`: the plate reads `chose copy`, and the same line is on stderr. |
//! | 7 | **Return chooses** | Reopen, press ↓ then Return: same result as 6 — the keyboard path and the pointer path end in the same callback. |
//! | 8 | **A disabled row** | `--disable-item 2`: `Delete` is greyed, arrow keys skip over it, and it cannot be clicked. |
//! | 9 | **A checked row** | `--flags selected`: `Copy as Markdown` carries the system tick in the system's own gutter — `--check-item` moves which row. |
//! | 10 | **A separator** | The rule above `Delete` is drawn by `AppKit`; it takes no highlight and no click. |
//! | 11 | **Submenu opens** | `--submenu`: hover `Copy as…` and wait — it opens on `AppKit`'s own delay, and → opens it from the keyboard. |
//! | 12 | **Submenu closes** | Move diagonally from `Copy as…` towards the submenu's far corner: it stays open (`AppKit`'s triangle heuristic), and ← closes it. |
//! | 13 | **Screen-edge flipping** | `--open corner`: the menu opens at the display's bottom-right and is flipped **up and to the left** so that no part of it is off-screen. |
//! | 14 | **`VoiceOver`** | ⌘F5, then open the menu: each row is announced with its enabled and checked state, and the submenu is announced as a submenu. |
//! | 15 | **An empty menu** | `--flags empty`: the popup opens with nothing in it and closes on Escape. `AppKit` draws this; it is not a crash. |
//! | 16 | **It closes when told** | `--open launch --dismiss-after 1500`: the menu opens without a click and closes itself 1.5s later, reporting `dismissed with nothing chosen`. |
//!
//! Number 16 is the one that runs without a human, and it exists because
//! synthetic pointer and keyboard events are denied on this project's machines
//! (`CGPreflightPostEventAccess()` is false). It proves the popup really opened
//! and really closed; it cannot prove anything about 2–14, and no automated
//! check on this machine can.
//!
//! # The state axis
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **real.** The tick on `--check-item`, drawn as `NSControlStateValueOn`. |
//! | `empty` | **real.** A menu with no items, which `AppKit` draws as a small empty popup. |
//! | `hover`, `focus` | **unmodelled, and that is the point of the item.** Which row is highlighted is `AppKit`'s business while it is tracking; nothing here can set it, and nothing here should. |
//! | `loading`, `error` | unmodelled, as on every surface. |
//!
//! # What the plate is, and is not
//!
//! It is driver chrome: a target to click and a readback of the last outcome.
//! It is not a ported component, which is why its anchor id is declared here
//! rather than in `crowbar-ui` — there is no React original for it, and a
//! `crowbar-ui` module for a thing with no original would be a thing the port
//! could not port.
//!
//! The root anchor exists so that the surface is coherent with the rest of the
//! registry, **not** so that it can be compared: a `CROWBAR_ROW_SNAPSHOT` run on
//! this surface emits the plate's box, which has no reference on the other side.

use std::cell::{Cell as StdCell, RefCell};
use std::fmt::Write as _;
use std::rc::Rc;
use std::time::Duration;

use crowbar_platform::native_menu::{ContextMenu, MenuError, MenuItem, ScreenPoint};
use crowbar_ui::Theme;
use crowbar_ui::components::{AnchorId, AnchorSink};
use gpui::{
    AnyElement, App, InteractiveElement as _, IntoElement, MouseButton, MouseDownEvent,
    ParentElement as _, Pixels, Point, SharedString, Styled as _, Window, canvas, div, relative,
};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, index, value};

/// The plate the menu is opened from, and this surface's root anchor.
pub const ID_TARGET: &str = "native-menu-target";

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "native-menu",
    root: ID_TARGET,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
    ],
    // The plate, its item list, the echo line and the caption below them. A
    // floor rather than a ceiling: this surface drives no height, because the
    // thing being judged is not in the window at all.
    min_window_height: 260,
    // A plate inside the window, like every other surface but `resizable`.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--check-item`'s default: the **second** row, so that a bare
/// `--flags selected` ticks a row that is neither the first nor the last and a
/// mis-numbering is visible rather than plausible.
pub const DEFAULT_CHECK_ITEM: usize = 1;

/// How far in from the display's corner `--open corner` opens, in points.
///
/// Not zero: a menu asked for at exactly (w, 0) is a menu asked for at a point
/// the cursor cannot occupy, and the flip being judged should be `AppKit`'s
/// ordinary one rather than its degenerate one.
const CORNER_INSET: f64 = 4.0;

/// What the plate says before anything has been opened.
const NOTHING_YET: &str = "no menu opened yet";

/// The last outcome, shared between the render that paints it and the task that
/// writes it.
type Echo = Rc<RefCell<SharedString>>;

/// When the menu opens.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Open {
    /// On a click on the plate, at the pointer. The real gesture.
    #[default]
    Click,
    /// On the first frame, at the centre of the driver's window.
    ///
    /// The mode that needs no pointer, and therefore the only one that runs on
    /// a machine where synthetic events are denied.
    Launch,
    /// On the first frame, at the primary display's bottom-right corner.
    ///
    /// The screen-edge flip, made reachable: the driver's window is small and
    /// sits at the top-left, so no click inside it could ever land near an edge
    /// the menu has to flip against.
    Corner,
}

impl Open {
    /// Its word on the command line and in the caption.
    #[must_use]
    pub const fn name(self) -> &'static str {
        match self {
            Self::Click => "click",
            Self::Launch => "launch",
            Self::Corner => "corner",
        }
    }

    /// Whether this mode opens the menu without waiting for a pointer.
    const fn unattended(self) -> bool {
        !matches!(self, Self::Click)
    }

    fn parse(raw: &str) -> Result<Self, ParseError> {
        match raw {
            "click" => Ok(Self::Click),
            "launch" => Ok(Self::Launch),
            "corner" => Ok(Self::Corner),
            other => Err(ParseError::Rejected(format!(
                "--open takes click, launch or corner, not {other}"
            ))),
        }
    }
}

/// This surface's own options, and the two live values the plate reads back.
#[derive(Clone, Debug)]
pub struct Params {
    /// `--check-item`: which row the `selected` flag ticks. Over the popup's
    /// **top-level** rows, so a separator and a submenu do not shift the
    /// numbering.
    pub check_item: usize,
    /// `--disable-item`: which row is `data-disabled`. A row parameter and not a
    /// §8.3 flag, exactly as it is on `dropdown-menu`.
    pub disable_item: Option<usize>,
    /// `--submenu`: append a `Copy as…` submenu, so that checks 11 and 12 have
    /// something to open.
    pub submenu: bool,
    /// `--open`.
    pub open: Open,
    /// `--dismiss-after`: close the menu from a timer, in milliseconds.
    ///
    /// The affordance that makes an unattended run mean something. See
    /// [`crowbar_platform::native_menu::cancel_tracking`] for why a block
    /// scheduled on the main queue reaches a menu that is already tracking.
    pub dismiss_after: Option<u64>,
    /// What the last menu did. **Not an option** — see this type's `PartialEq`.
    echo: Echo,
    /// Whether an unattended open has already fired, so that it fires once
    /// rather than once per frame. **Not an option**, for the same reason.
    armed: Rc<StdCell<bool>>,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            check_item: DEFAULT_CHECK_ITEM,
            disable_item: None,
            submenu: false,
            open: Open::default(),
            dismiss_after: None,
            echo: Rc::new(RefCell::new(SharedString::new_static(NOTHING_YET))),
            armed: Rc::new(StdCell::new(false)),
        }
    }
}

/// **The options, and only the options.**
///
/// Written out rather than derived because two of the fields are live state:
/// what the last menu did, and whether the unattended open has fired. A derived
/// `PartialEq` would make two cells describing the same picture compare unequal
/// as soon as somebody clicked one of them, which is a fact about the session
/// rather than about the cell.
impl PartialEq for Params {
    fn eq(&self, other: &Self) -> bool {
        self.check_item == other.check_item
            && self.disable_item == other.disable_item
            && self.submenu == other.submenu
            && self.open == other.open
            && self.dismiss_after == other.dismiss_after
    }
}

impl Params {
    /// The menu this cell describes.
    ///
    /// The fixture is the live comment-actions menu from
    /// `web/src/features/git/components/review-thread-item.tsx` — the same one
    /// `dropdown-menu` ports — so the two surfaces are two renderings of one
    /// menu and can be looked at side by side.
    #[must_use]
    pub fn menu(&self, cell: &Cell) -> ContextMenu {
        if cell.has(StateFlag::Empty) {
            // A menu with nothing in it, which is a picture `AppKit` has.
            return ContextMenu::new();
        }

        let mut menu = self.row(ContextMenu::new(), 0, "edit", "Edit", cell);
        menu = self.row(menu, 1, "copy", "Copy as Markdown", cell);
        if self.submenu {
            menu = menu.submenu(
                "Copy as…",
                ContextMenu::new()
                    .item("copy-markdown", "Markdown")
                    .item("copy-plain", "Plain Text"),
            );
        }
        self.row(menu.separator(), 2, "delete", "Delete", cell)
    }

    /// One top-level row, with this cell's tick and disabled state applied.
    fn row(
        &self,
        menu: ContextMenu,
        position: usize,
        id: &str,
        title: &str,
        cell: &Cell,
    ) -> ContextMenu {
        if self.disable_item == Some(position) {
            return menu.disabled_item(id, title);
        }
        let ticked = cell.has(StateFlag::Selected) && self.check_item == position;
        menu.checked_item(id, title, ticked)
    }

    /// The menu, written out the way the plate shows it — so that what was
    /// handed to `AppKit` can be read off the window and compared with what
    /// `AppKit` drew.
    #[must_use]
    pub fn lines(&self, cell: &Cell) -> Vec<SharedString> {
        let menu = self.menu(cell);
        if menu.items.is_empty() {
            return vec![SharedString::new_static("(no items)")];
        }
        let mut lines = Vec::new();
        describe_items(&menu.items, 0, &mut lines);
        lines
    }
}

/// Every item under `items`, one line each, indented by nesting depth.
fn describe_items(items: &[MenuItem], depth: usize, out: &mut Vec<SharedString>) {
    let pad = "    ".repeat(depth);
    for item in items {
        match item {
            MenuItem::Separator => out.push(SharedString::from(format!("{pad}  ────────"))),
            MenuItem::Item {
                title,
                enabled,
                checked,
                ..
            } => {
                let tick = if *checked { "✓" } else { " " };
                let state = if *enabled { "" } else { "   (disabled)" };
                out.push(SharedString::from(format!("{pad}{tick} {title}{state}")));
            }
            MenuItem::Submenu {
                title,
                enabled,
                items,
            } => {
                let state = if *enabled { "" } else { "   (disabled)" };
                out.push(SharedString::from(format!("{pad}  {title} ▸{state}")));
                describe_items(items, depth + 1, out);
            }
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--check-item" => self.check_item = index(&value(args, option)?, option)?,
            "--disable-item" => self.disable_item = Some(index(&value(args, option)?, option)?),
            "--submenu" => self.submenu = true,
            "--open" => self.open = Open::parse(&value(args, option)?)?,
            "--dismiss-after" => {
                let raw = value(args, option)?;
                self.dismiss_after = Some(raw.parse().map_err(|_| {
                    ParseError::Rejected(format!(
                        "--dismiss-after takes a whole number of milliseconds, not {raw}"
                    ))
                })?);
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The thing this surface is about is not in the window: an
    /// `NSMenu` is as tall as its rows and is positioned by `AppKit` against the
    /// display, not against the driver's window. Nothing on the command line
    /// decides the plate's height either — it is its content's — so this surface
    /// keeps its floor.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · open on {}", self.open.name());
        if let Some(ms) = self.dismiss_after {
            let _ = write!(out, " · dismiss after {ms}ms");
        }
        if cell.has(StateFlag::Selected) {
            let _ = write!(out, " · tick row {}", self.check_item);
        }
        if let Some(row) = self.disable_item {
            let _ = write!(out, " · disabled row {row}");
        }
        if self.submenu {
            out.push_str(" · submenu");
        }
        // Said on every caption, because it is the single largest thing a reader
        // could otherwise assume this surface covers.
        out.push_str(" · no geometry gate: the popup is an NSMenu and no extractor can see it");
        // Per cell rather than per surface: `--theme` picks the token table the
        // *plate* is drawn from, and the menu ignores it entirely.
        out.push_str(" · the menu takes the system appearance, not --theme");
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let menu = self.menu(cell);
        let echo = self
            .echo
            .try_borrow()
            .map_or_else(|_| SharedString::new_static(NOTHING_YET), |it| it.clone());

        let mut plate = div()
            .flex()
            .flex_col()
            .gap_1()
            .p_3()
            .rounded(theme.radius_lg.value())
            .bg(theme.popover)
            .border_1()
            .border_color(theme.border)
            .text_color(theme.popover_foreground)
            .text_size(theme.ui_text_base.value())
            .line_height(relative(1.35))
            .child(
                div()
                    .text_size(theme.ui_text_xs.value())
                    .text_color(theme.muted_foreground)
                    .child(match self.open {
                        Open::Click => "right-click (or click) here for the native menu",
                        Open::Launch => "the native menu opens by itself at this window's centre",
                        Open::Corner => "the native menu opens by itself at the display's corner",
                    }),
            )
            .children(
                self.lines(cell)
                    .into_iter()
                    .map(|line| div().pl_2().child(line)),
            )
            .child(
                div()
                    .pt_1()
                    .text_size(theme.ui_text_xs.value())
                    .text_color(theme.muted_foreground)
                    .child(echo),
            )
            .child(self.launcher(menu.clone()));

        for button in [MouseButton::Right, MouseButton::Left] {
            plate = plate.on_mouse_down(
                button,
                listener(menu.clone(), Rc::clone(&self.echo), self.dismiss_after),
            );
        }

        anchors.root(AnchorId::from(ID_TARGET), plate)
    }
}

impl Params {
    /// A zero-sized element whose prepaint opens the menu once, for the two
    /// `--open` modes that need no pointer.
    ///
    /// A `canvas` rather than anything on the view, because a surface's
    /// `render` is handed a theme and an anchor sink and no `Window` —
    /// deliberately, since a surface is a picture rather than a program. This is
    /// the one hook that reaches the frame without changing that.
    fn launcher(&self, menu: ContextMenu) -> impl IntoElement {
        let echo = Rc::clone(&self.echo);
        let armed = Rc::clone(&self.armed);
        let mode = self.open;
        let dismiss_after = self.dismiss_after;

        canvas(
            move |_bounds, window, cx| {
                // Prepaint runs every frame, and the menu closing schedules
                // another one; without the latch a `--open launch` run would
                // reopen the menu for as long as the app was up.
                if !mode.unattended() || armed.replace(true) {
                    return;
                }
                let at = match mode {
                    Open::Corner => display_corner(cx),
                    Open::Click | Open::Launch => window_centre(window, cx),
                };
                open(menu, at, echo, dismiss_after, window, cx);
            },
            |_bounds, (), _window, _cx| {},
        )
    }
}

/// The mouse-down listener: open the menu where the pointer is.
fn listener(
    menu: ContextMenu,
    echo: Echo,
    dismiss_after: Option<u64>,
) -> impl Fn(&MouseDownEvent, &mut Window, &mut App) + 'static {
    move |event, window, cx| {
        let at = screen_point(event.position, window, cx);
        open(
            menu.clone(),
            at,
            Rc::clone(&echo),
            dismiss_after,
            window,
            cx,
        );
    }
}

/// Shows the menu, off the caller's stack, and records what came back.
///
/// **Not inline in the event handler.** `show_at` runs `AppKit`'s own tracking
/// loop, which does not return until the menu closes; running it from inside a
/// GPUI event handler would hold the app borrowed across a nested run loop for
/// as long as a user held the menu open. A foreground task is the same thread —
/// which `NSMenu` requires — with nothing borrowed.
fn open(
    menu: ContextMenu,
    at: ScreenPoint,
    echo: Echo,
    dismiss_after: Option<u64>,
    window: &Window,
    cx: &mut App,
) {
    // Where, on stderr, before anything is shown: check 13 is a claim about the
    // point the menu was asked for and where `AppKit` then put it, and half of
    // that is not observable from a screenshot.
    eprintln!("crowbar-app: native-menu opening at ({}, {})", at.x, at.y);

    let handle = window.window_handle();
    cx.spawn(async move |cx| {
        // Armed on the main thread, from the same task, immediately before the
        // menu opens — and *not* from a `background_executor().timer()`, which
        // was tried first and does not work: a `GPUI` foreground task runs
        // inside a main-dispatch-queue block, and `libdispatch` will not begin
        // another one while that block is on the stack, however long the menu's
        // nested run loop spins. See
        // `crowbar_platform::native_menu::cancel_tracking_after`.
        if let Some(ms) = dismiss_after
            && let Err(err) =
                crowbar_platform::native_menu::cancel_tracking_after(Duration::from_millis(ms))
        {
            eprintln!("crowbar-app: native-menu could not arm its dismissal: {err}");
        }

        let line = outcome(menu.show_at(at));
        // On stderr as well as on the plate: an unattended run has no eyes on
        // the window, and this is the line it is judged by.
        eprintln!("crowbar-app: native-menu {line}");
        cx.update(|app| {
            let _ = handle.update(app, |_, window, _| {
                if let Ok(mut slot) = echo.try_borrow_mut() {
                    *slot = SharedString::from(line);
                }
                // The window is idle while `AppKit` tracks, and a dismissed menu
                // leaves it that way; this is what paints the echo and puts the
                // mouse handlers back.
                window.refresh();
            });
        });
    })
    .detach();
}

/// What a completed menu did, in the words the plate and stderr both use.
fn outcome(result: Result<Option<&str>, MenuError>) -> String {
    match result {
        Ok(Some(id)) => format!("chose {id}"),
        Ok(None) => "dismissed with nothing chosen".to_owned(),
        Err(err) => format!("refused: {err}"),
    }
}

/// A window-relative GPUI point, as the screen point `AppKit` wants.
fn screen_point(local: Point<Pixels>, window: &Window, cx: &App) -> ScreenPoint {
    let origin = window.bounds().origin;
    ScreenPoint::from_top_left(
        logical(origin.x + local.x),
        logical(origin.y + local.y),
        primary_size(cx).1,
    )
}

/// The centre of the driver's window, as a screen point.
fn window_centre(window: &Window, cx: &App) -> ScreenPoint {
    let bounds = window.bounds();
    screen_point(
        Point {
            x: bounds.size.width / 2.0,
            y: bounds.size.height / 2.0,
        },
        window,
        cx,
    )
}

/// The primary display's bottom-right corner, [`CORNER_INSET`] in on both axes.
///
/// In `AppKit`'s own space, so there is no flip here to get wrong: y counts up
/// from the bottom, and the bottom is where the menu has to be pushed away from.
fn display_corner(cx: &App) -> ScreenPoint {
    let (width, _) = primary_size(cx);
    ScreenPoint::new(width - CORNER_INSET, CORNER_INSET)
}

/// The primary display's size in points, or zeroes if there is no display —
/// which on a machine with a window open there is not, but a `map_or` is a
/// smaller thing to be wrong about than an `expect`.
fn primary_size(cx: &App) -> (f64, f64) {
    cx.primary_display().map_or((0.0, 0.0), |display| {
        let size = display.bounds().size;
        (logical(size.width), logical(size.height))
    })
}

/// Logical pixels as the `f64` `CGFloat` is.
fn logical(pixels: Pixels) -> f64 {
    f64::from(f32::from(pixels))
}

fn options() -> Vec<(String, String)> {
    [
        (
            "--check-item <n>".to_owned(),
            format!("which top-level row `selected` ticks [{DEFAULT_CHECK_ITEM}]"),
        ),
        (
            "--disable-item <n>".to_owned(),
            "data-disabled on one top-level row [none]".to_owned(),
        ),
        (
            "--submenu".to_owned(),
            "append a `Copy as…` submenu, so submenu behaviour can be judged".to_owned(),
        ),
        (
            "--open click|launch|corner".to_owned(),
            "when and where the menu opens; corner exercises screen-edge flipping [click]"
                .to_owned(),
        ),
        (
            "--dismiss-after <ms>".to_owned(),
            "close the menu from a timer; the one check that needs no human [none]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_CHECK_ITEM, ID_TARGET, Open, Params, SURFACE, options, outcome};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_platform::native_menu::{MenuError, MenuItem};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "native-menu"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a native-menu cell carries this surface's bag")
    }

    fn menu_of(cell: &Cell) -> Vec<MenuItem> {
        params_of(cell).menu(cell).items
    }

    /// The ids of the top-level rows this cell ticks.
    fn ticked_ids(cell: &Cell) -> Vec<String> {
        menu_of(cell)
            .into_iter()
            .filter_map(|item| match item {
                MenuItem::Item {
                    id, checked: true, ..
                } => Some(id),
                _ => None,
            })
            .collect()
    }

    /// The fixture is the live comment-actions menu, so a bare
    /// `--surface native-menu` opens the menu the reference actually has — and
    /// `dropdown-menu` renders the same one.
    #[test]
    fn the_default_menu_is_the_comment_menu() {
        let bag = Params::default();
        assert_eq!(bag.check_item, DEFAULT_CHECK_ITEM);
        assert_eq!(bag.disable_item, None);
        assert!(!bag.submenu);
        assert_eq!(bag.open, Open::Click);
        assert_eq!(bag.dismiss_after, None);

        let resting = cell(&[]);
        assert_eq!(
            params_of(&resting).menu(&resting).chosen_ids(),
            ["edit", "copy", "delete"],
        );
        assert!(matches!(menu_of(&resting)[2], MenuItem::Separator));
        // Nothing is ticked and nothing is disabled at rest.
        for item in menu_of(&resting) {
            if let MenuItem::Item {
                enabled, checked, ..
            } = item
            {
                assert!(enabled);
                assert!(!checked);
            }
        }
    }

    /// `selected` is the tick, `--check-item` is which row, and the flag is what
    /// decides whether anything is ticked at all — two knobs for one picture, as
    /// `dropdown-menu` splits `--focus-row` from `--flags focus`.
    #[test]
    fn selected_ticks_the_check_item_and_nothing_else() {
        for (position, id) in [(0_usize, "edit"), (1, "copy"), (2, "delete")] {
            let ticked = cell(&["--flags", "selected", "--check-item", &position.to_string()]);
            assert_eq!(ticked_ids(&ticked), [id.to_owned()], "row {position}");

            // Without the flag the same `--check-item` ticks nothing.
            let resting = cell(&["--check-item", &position.to_string()]);
            assert!(
                menu_of(&resting)
                    .iter()
                    .all(|item| !matches!(item, MenuItem::Item { checked: true, .. })),
                "row {position}",
            );
        }
    }

    /// A disabled row keeps its id and its place: it is still in the tag table,
    /// because `--disable-item` must not renumber the rows around it.
    #[test]
    fn a_disabled_row_keeps_its_place_in_the_numbering() {
        for position in 0_usize..3 {
            let disabled = cell(&["--disable-item", &position.to_string()]);
            let menu = params_of(&disabled).menu(&disabled);

            assert_eq!(menu.chosen_ids(), ["edit", "copy", "delete"], "{position}");

            let enabled: Vec<bool> = menu
                .items
                .iter()
                .filter_map(|item| match item {
                    MenuItem::Item { enabled, .. } => Some(*enabled),
                    _ => None,
                })
                .collect();
            let mut expected = [true; 3];
            expected[position] = false;
            assert_eq!(enabled, expected, "{position}");
        }
    }

    /// **A disabled row is never ticked**, even when `--check-item` names it:
    /// the two options would otherwise describe one row two ways, and `AppKit`
    /// would draw a tick on a row nobody can reach.
    #[test]
    fn a_disabled_row_is_not_also_ticked() {
        let both = cell(&[
            "--flags",
            "selected",
            "--check-item",
            "1",
            "--disable-item",
            "1",
        ]);

        for item in menu_of(&both) {
            if let MenuItem::Item {
                enabled, checked, ..
            } = item
            {
                assert!(!checked || enabled, "a ticked row must be reachable");
            }
        }
    }

    /// `--submenu` adds a nested menu **without** shifting the top-level
    /// numbering, which is what makes `--check-item` mean the same thing with
    /// and without one.
    #[test]
    fn a_submenu_does_not_shift_the_row_numbering() {
        let with = cell(&["--submenu", "--flags", "selected", "--check-item", "2"]);
        let menu = params_of(&with).menu(&with);

        assert_eq!(
            menu.chosen_ids(),
            ["edit", "copy", "copy-markdown", "copy-plain", "delete"],
        );
        assert!(
            menu.items
                .iter()
                .any(|item| matches!(item, MenuItem::Submenu { .. })),
        );

        // And the tick is on the third **top-level** row rather than on the
        // third entry of the flattened list, which is `copy-markdown`.
        assert_eq!(ticked_ids(&with), ["delete".to_owned()]);
        assert_eq!(menu.chosen_ids()[2], "copy-markdown");
    }

    /// `empty` is a real state here: `AppKit` draws a popup with nothing in it,
    /// so the cell can fail.
    #[test]
    fn the_empty_flag_is_a_menu_with_no_items() {
        let empty = cell(&["--flags", "empty"]);

        assert!(params_of(&empty).menu(&empty).items.is_empty());
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        // The plate still says something rather than drawing nothing.
        assert_eq!(params_of(&empty).lines(&empty), ["(no items)"]);
    }

    /// **The two flags `AppKit` owns.** Which row is highlighted is decided
    /// inside the tracking loop, and nothing on this side can set it — which is
    /// half the argument for using `NSMenu` at all.
    #[test]
    fn the_highlight_flags_are_appkits_and_are_declared_unmodelled() {
        for flag in [
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Loading,
            StateFlag::Error,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }

        // And driving one changes nothing about the menu, which is what
        // "unmodelled" has to mean if the word is to be worth anything.
        let resting = cell(&[]);
        for flag in ["hover", "focus", "loading", "error"] {
            let driven = cell(&["--flags", flag]);
            assert_eq!(
                params_of(&driven).menu(&driven),
                params_of(&resting).menu(&resting),
                "{flag}",
            );
        }
    }

    /// The vocabulary is closed and every rejection names what was wanted.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--open", "hover"],
            vec!["--open"],
            vec!["--check-item", "second"],
            vec!["--check-item"],
            vec!["--disable-item", "-1"],
            vec!["--disable-item"],
            vec!["--dismiss-after", "soon"],
            vec!["--dismiss-after"],
            vec!["--dismiss-after", "-1"],
        ] {
            let mut full = vec!["--surface", "native-menu"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "native-menu", "--open", "hover"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("`hover` is not an open mode");
        };
        for word in ["click", "launch", "corner"] {
            assert!(complaint.contains(word), "{complaint}");
        }
    }

    /// **These options belong to this surface and to no other.**
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in ["--check-item", "--disable-item", "--submenu", "--open"] {
            let line = ["--surface", "git-status-row", option, "1"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a git-status-row option",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in [
            "--check-item",
            "--disable-item",
            "--submenu",
            "--open",
            "--dismiss-after",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("native-menu"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
        // The usage lists which flags each surface really has; this one has
        // `empty` and `selected` and no others.
        assert!(usage.contains("corner"), "{usage}");
    }

    /// The caption says what the run will do, and — on every cell — the two
    /// things a reader would otherwise assume: there is no geometry gate, and
    /// `--theme` does not reach the menu.
    #[test]
    fn the_caption_says_what_this_surface_does_not_measure() {
        for line in [
            vec![],
            vec!["--theme", "light"],
            vec!["--flags", "selected"],
        ] {
            let caption = cell(&line).describe();
            assert!(caption.contains("no geometry gate"), "{caption}");
            assert!(caption.contains("system appearance"), "{caption}");
        }

        let unattended = cell(&["--open", "corner", "--dismiss-after", "1500"]);
        let caption = unattended.describe();
        assert!(caption.contains("open on corner"), "{caption}");
        assert!(caption.contains("dismiss after 1500ms"), "{caption}");

        let ticked = cell(&["--flags", "selected", "--check-item", "2", "--submenu"]);
        let caption = ticked.describe();
        assert!(caption.contains("tick row 2"), "{caption}");
        assert!(caption.contains("submenu"), "{caption}");
    }

    /// Every outcome the driver can reach says which one it is, in words a human
    /// judging by eye can check against the menu they just closed.
    #[test]
    fn every_outcome_is_reported_in_one_line() {
        assert_eq!(outcome(Ok(Some("delete"))), "chose delete");
        assert_eq!(outcome(Ok(None)), "dismissed with nothing chosen");

        let refused = outcome(Err(MenuError::OffMainThread));
        assert!(refused.starts_with("refused: "), "{refused}");
        assert!(refused.contains("main thread"), "{refused}");
    }

    /// The two live fields are not part of the cell's identity: a cell that has
    /// been clicked describes the same picture as one that has not.
    #[test]
    fn the_live_readback_is_not_part_of_the_cells_identity() {
        let one = Params::default();
        let mut other = Params::default();

        assert_eq!(one, other);
        other.armed.set(true);
        assert_eq!(one, other, "the latch is session state, not a parameter");

        other.check_item = 2;
        assert_ne!(one, other, "an option is");
    }

    /// The registry entry's two contract fields, and the fact that this
    /// surface's root is its plate rather than the popup — because no anchor
    /// sink can reach an `NSMenu`.
    #[test]
    fn the_surface_names_itself_and_anchors_its_trigger() {
        assert_eq!(SURFACE.name, "native-menu");
        assert_eq!(SURFACE.root, ID_TARGET);
        assert_eq!(SURFACE.root, "native-menu-target");
        assert_eq!(SURFACE.min_window_height, 260);
        assert!(!SURFACE.full_bleed);
    }

    /// The mode words are closed, and each one is spelled the same on the
    /// command line and in the caption.
    #[test]
    fn every_open_mode_round_trips_through_its_word() {
        for mode in [Open::Click, Open::Launch, Open::Corner] {
            let driven = cell(&["--open", mode.name()]);
            assert_eq!(params_of(&driven).open, mode);
            assert!(driven.describe().contains(mode.name()), "{}", mode.name());
        }
        assert!(!Open::Click.unattended());
        assert!(Open::Launch.unattended());
        assert!(Open::Corner.unattended());
    }
}
