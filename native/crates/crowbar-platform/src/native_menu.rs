//! A **real macOS context menu**: `NSMenu`, shown at a screen point, returning
//! the item that was chosen.
//!
//! # Why the OS draws this one
//!
//! Crowbar's dropdown menus are native rather than simulated. That is a product
//! ruling, and it has a technical consequence worth writing down: an `NSMenu`
//! cannot be anchor-diffed against a DOM popup, so this surface leaves the
//! strict-parity gate. What it buys instead is everything a re-implementation
//! would have to earn one behaviour at a time — arrow-key navigation, type-select,
//! Escape, click-outside, submenu open/close timing, screen-edge flipping,
//! `VoiceOver`, the user's own menu tint and accessibility settings — supplied by
//! `AppKit` and correct by construction.
//!
//! **This is for context menus, and only for them.** Anything that must carry
//! Crowbar's design tokens, or live inside a pane, stays a GPUI-drawn popup;
//! `crowbar_ui::components::dropdown_menu` is that, and it is not superseded by
//! this module. An `NSMenu` takes the system's appearance, not `theme.css`'s.
//!
//! # The vocabulary
//!
//! [`MenuItem`] is the subset of `web/src/components/ui/dropdown-menu.tsx` that
//! `AppKit` can express, and the mapping is deliberately narrow:
//!
//! | `dropdown-menu.tsx` | here | `AppKit` |
//! |---|---|---|
//! | `menu-item` | [`MenuItem::Item`] | `NSMenuItem` with a target and an action |
//! | `menu-separator` | [`MenuItem::Separator`] | `+[NSMenuItem separatorItem]` |
//! | `menu-checkbox-item` | `MenuItem::Item { checked: true, .. }` | `-setState:NSControlStateValueOn` |
//! | `data-disabled` | `MenuItem::Item { enabled: false, .. }` | `-setEnabled:NO`, no action |
//! | `menu-sub-trigger` + `menu-sub-popup` | [`MenuItem::Submenu`] | `-setSubmenu:` |
//!
//! What is **not** here, stated rather than discovered at a call site.
//! `menu-radio-item` has no `AppKit` primitive of its own — a radio group is a set
//! of items whose ticks the *application* keeps mutually exclusive — so it is an
//! `Item` with `checked` and the caller owns the exclusivity.
//! `menu-label`, `menu-shortcut` and `inset` are presentation the platform
//! decides: a section header is a disabled item, a key equivalent is the
//! responder chain's business, and `AppKit` lays out its own gutter.
//!
//! # Main thread only
//!
//! `NSMenu` and `NSMenuItem` are `MainThreadOnly` classes. Every `AppKit` call in
//! this module sits behind an `objc2::MainThreadMarker`, which can only be
//! obtained by asking the runtime — so the rule is enforced by the *type system*
//! at every call site, and there is no path that reaches `AppKit` without one.
//!
//! Off the main thread [`ContextMenu::show_at`] and [`cancel_tracking`] return
//! [`MenuError::OffMainThread`] and touch nothing at all: no allocation, no
//! message send, no window. **That is a refusal, not a panic** — a background
//! thread that asks for a menu has made a mistake the caller can report, and a
//! panic across an Objective-C frame would be worse than the mistake.
//!
//! # Coordinates
//!
//! [`ScreenPoint`] is in **`AppKit` screen coordinates**: points, origin at the
//! bottom-left of the primary display, y increasing upwards. That is the space
//! `-[NSMenu popUpMenuPositioningItem:atLocation:inView:]` reads when the view is
//! `nil`, so it is the space this API takes, rather than a friendlier one that
//! would have to be un-converted immediately.
//!
//! Callers living in a y-down space — GPUI's global coordinates, and every
//! browser — convert with [`ScreenPoint::from_top_left`], which is here rather
//! than at the call site because the convention is a fact about `AppKit` and
//! belongs next to the code that depends on it.
//!
//! # Testing it on a machine that cannot click
//!
//! Synthetic pointer and keyboard events are denied to this project's agents
//! (`CGPreflightPostEventAccess()` is false), so a test that drove the menu with
//! a click could not run. The selection path is therefore split so that the part
//! worth testing does not need one: [`Selection`] is the whole body of the
//! Objective-C action callback, it is public, and a test can invoke
//! [`Selection::record`] exactly as `AppKit` would and then resolve it through
//! [`ContextMenu::chosen`]. What that leaves untested by a unit test is the
//! message send itself, and no unit test on any machine could have covered that.

use std::fmt;

#[cfg(target_os = "macos")]
mod appkit;

/// A point in **`AppKit` screen coordinates**: points, origin at the bottom-left
/// of the primary display, y increasing upwards.
///
/// `f64` because that is `CGFloat` on every Mac this port targets, so the value
/// crosses the boundary without a conversion that could round.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct ScreenPoint {
    /// Distance from the primary display's left edge.
    pub x: f64,
    /// Distance **up** from the primary display's bottom edge.
    pub y: f64,
}

impl ScreenPoint {
    /// A point already in `AppKit`'s space.
    #[must_use]
    pub const fn new(x: f64, y: f64) -> Self {
        Self { x, y }
    }

    /// The same point given in a y-**down** space whose origin is the top-left
    /// of the primary display — GPUI's global coordinates, and the browser's.
    ///
    /// `primary_display_height` is the primary display's full height in points
    /// (`gpui::App::primary_display()`'s `bounds().size.height`), because that
    /// is the quantity the two spaces are reflections of each other about.
    ///
    /// **Exact on one display; an approximation on several.** A second display
    /// stacked above or below the primary one has `AppKit` screen coordinates that
    /// run past the primary display's own edges in both directions, and a single
    /// height cannot describe that. Nothing here can fix it without asking
    /// `AppKit` which screen the point is on, which would make a pure function
    /// main-thread-only; a caller that needs multi-display exactness should
    /// compute the `AppKit` point itself and use [`ScreenPoint::new`].
    #[must_use]
    pub fn from_top_left(x: f64, y: f64, primary_display_height: f64) -> Self {
        Self {
            x,
            y: primary_display_height - y,
        }
    }
}

/// One entry in a [`ContextMenu`].
///
/// A plain data enum, so a menu can be built as a value, compared, logged, and
/// asserted on without an `AppKit` object existing. The `AppKit` objects are made
/// inside [`ContextMenu::show_at`] and destroyed before it returns.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum MenuItem {
    /// `menu-item`, and — with `checked` — `menu-checkbox-item`.
    ///
    /// The only kind that can be chosen, and therefore the only kind that
    /// carries an `id`.
    Item {
        /// What [`ContextMenu::show_at`] returns when this row is chosen.
        ///
        /// The caller's own word. Nothing here interprets it, and duplicates are
        /// permitted but useless: the first row with that id is the one a
        /// resolved selection is indistinguishable from.
        id: String,
        /// The row's text.
        title: String,
        /// `false` is `data-disabled`: greyed, unselectable, and given no action
        /// at all rather than an action that is merely ignored.
        enabled: bool,
        /// `true` draws `AppKit`'s tick — `NSControlStateValueOn`.
        checked: bool,
    },
    /// `menu-separator`.
    Separator,
    /// `menu-sub-trigger` and the `menu-sub-popup` it opens.
    Submenu {
        /// The parent row's text. `AppKit` draws the disclosure arrow itself.
        title: String,
        /// `false` greys the parent row and the submenu never opens.
        enabled: bool,
        /// The submenu's own entries. Nesting is not limited here; `AppKit`'s
        /// depth limit is the one that applies.
        items: Vec<MenuItem>,
    },
}

/// A native context menu: what to show, and what came back.
///
/// Built with the chaining constructors below, or as data through
/// [`ContextMenu::items`]:
///
/// ```
/// use crowbar_platform::native_menu::ContextMenu;
///
/// let menu = ContextMenu::new()
///     .item("edit", "Edit")
///     .item("copy", "Copy as Markdown")
///     .separator()
///     .item("delete", "Delete");
///
/// assert_eq!(menu.chosen_ids(), ["edit", "copy", "delete"]);
/// ```
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct ContextMenu {
    /// The entries, in the order they are shown.
    pub items: Vec<MenuItem>,
}

impl ContextMenu {
    /// An empty menu.
    ///
    /// Empty is a legitimate thing to show — `AppKit` draws a small empty popup —
    /// and it is what the driver surface's `empty` cell renders, so it is not
    /// refused here.
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// Appends an enabled, unticked `menu-item`.
    #[must_use]
    pub fn item(self, id: impl Into<String>, title: impl Into<String>) -> Self {
        self.with(MenuItem::Item {
            id: id.into(),
            title: title.into(),
            enabled: true,
            checked: false,
        })
    }

    /// Appends a `menu-checkbox-item`, ticked or not.
    #[must_use]
    pub fn checked_item(
        self,
        id: impl Into<String>,
        title: impl Into<String>,
        checked: bool,
    ) -> Self {
        self.with(MenuItem::Item {
            id: id.into(),
            title: title.into(),
            enabled: true,
            checked,
        })
    }

    /// Appends a `data-disabled` item.
    ///
    /// It keeps its id and its place in the tag numbering — see
    /// [`ContextMenu::chosen_ids`] — because a row that can be enabled by a
    /// later state change must not renumber the rows around it.
    #[must_use]
    pub fn disabled_item(self, id: impl Into<String>, title: impl Into<String>) -> Self {
        self.with(MenuItem::Item {
            id: id.into(),
            title: title.into(),
            enabled: false,
            checked: false,
        })
    }

    /// Appends a `menu-separator`.
    #[must_use]
    pub fn separator(self) -> Self {
        self.with(MenuItem::Separator)
    }

    /// Appends a submenu.
    #[must_use]
    pub fn submenu(self, title: impl Into<String>, items: Self) -> Self {
        self.with(MenuItem::Submenu {
            title: title.into(),
            enabled: true,
            items: items.items,
        })
    }

    /// Appends any [`MenuItem`], for the shapes the chaining constructors do not
    /// spell.
    #[must_use]
    pub fn with(mut self, item: MenuItem) -> Self {
        self.items.push(item);
        self
    }

    /// Every choosable row's id, depth-first, in the order `AppKit` tags are
    /// assigned.
    ///
    /// **Disabled rows are in this list.** A tag is an index into the menu's
    /// *shape*, not into the subset that happens to be selectable right now, so
    /// enabling or disabling a row cannot silently make some other row's tag mean
    /// a different id. A disabled row is simply never the one `AppKit` reports.
    ///
    /// Separators are not in it — they have no id — and neither is a submenu's
    /// parent row: choosing it opens the submenu rather than dismissing the menu.
    #[must_use]
    pub fn chosen_ids(&self) -> Vec<&str> {
        let mut ids = Vec::new();
        collect_ids(&self.items, &mut ids);
        ids
    }

    /// The id an `AppKit` tag names, or `None` for a tag this menu has no row for.
    ///
    /// Negative and out-of-range tags are both `None` rather than a panic: the
    /// tag arrives from Objective-C, where any `NSInteger` is representable, and
    /// a menu that had been rebuilt between the show and the resolve would
    /// otherwise be a crash instead of a miss.
    #[must_use]
    pub fn id_for_tag(&self, tag: isize) -> Option<&str> {
        let index = usize::try_from(tag).ok()?;
        self.chosen_ids().into_iter().nth(index)
    }

    /// The id a [`Selection`] names, or `None` if nothing was chosen.
    ///
    /// The seam that makes the selection path testable without a click: hand it
    /// a `Selection` that a test wrote with [`Selection::record`] and it answers
    /// exactly as it does for one `AppKit` wrote.
    #[must_use]
    pub fn chosen(&self, selection: &Selection) -> Option<&str> {
        self.id_for_tag(selection.taken()?)
    }

    /// Shows the menu at `at` and **blocks until it closes**, answering with the
    /// id of the row that was chosen, or `None` if it was dismissed.
    ///
    /// Blocking is `AppKit`'s design, not a shortcut taken here:
    /// `-popUpMenuPositioningItem:atLocation:inView:` runs its own event-tracking
    /// loop and returns when tracking ends. A caller on a run loop it does not
    /// want stalled — a GPUI window, say — should reach this from a task rather
    /// than from inside an event handler, so that nothing is borrowed across the
    /// nested loop. The main dispatch queue *is* serviced while tracking, which
    /// is why [`cancel_tracking`] can be scheduled from outside and still run.
    ///
    /// # Errors
    ///
    /// [`MenuError::OffMainThread`] when called from anywhere but the main
    /// thread; [`MenuError::AlreadyTracking`] when this process is already
    /// showing a menu; [`MenuError::Unsupported`] on a platform with no
    /// implementation. In every case nothing is shown and nothing is allocated.
    pub fn show_at(&self, at: ScreenPoint) -> Result<Option<&str>, MenuError> {
        #[cfg(target_os = "macos")]
        {
            let selection = appkit::show(&self.items, at)?;
            Ok(self.chosen(&selection))
        }
        #[cfg(not(target_os = "macos"))]
        {
            // Named rather than dropped, so that the signature is the API on
            // every target and neither parameter reads as an oversight here.
            let _ = (self, at);
            Err(MenuError::Unsupported)
        }
    }
}

/// Every choosable id under `items`, appended depth-first.
///
/// Free rather than a method so that the recursion is over a slice: a submenu's
/// entries are a `Vec<MenuItem>` and not a [`ContextMenu`], because a submenu is
/// not separately showable.
fn collect_ids<'a>(items: &'a [MenuItem], out: &mut Vec<&'a str>) {
    for item in items {
        match item {
            MenuItem::Item { id, .. } => out.push(id),
            MenuItem::Submenu { items, .. } => collect_ids(items, out),
            MenuItem::Separator => {}
        }
    }
}

/// Where the menu's action callback writes the tag of the row that was chosen.
///
/// # Why this is public
///
/// It is the entire body of the Objective-C method `NSMenuItem` sends when a row
/// is picked. Making it public is what lets the selection path be tested by
/// invoking the callback directly — which on this project's machines is the only
/// way it *can* be tested, synthetic pointer and keyboard events being denied.
/// A test that calls [`Selection::record`] is exercising the same line `AppKit`
/// does, not a stand-in for it.
///
/// Not `Sync`, and it must not become so: it is written from the main thread
/// inside a tracking loop and read from the main thread after it.
///
/// `Clone` is a **snapshot**, not a shared handle. It exists because the live
/// selection lives in an Objective-C instance variable that dies with the menu,
/// and the answer has to outlive it.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Selection {
    /// The chosen tag, or [`Selection::NOTHING`].
    tag: std::cell::Cell<isize>,
}

impl Selection {
    /// The tag that means "the menu was dismissed".
    ///
    /// Negative, so that it cannot collide with a row: tags are indices.
    pub const NOTHING: isize = -1;

    /// A selection holding nothing.
    #[must_use]
    pub fn new() -> Self {
        Self {
            tag: std::cell::Cell::new(Self::NOTHING),
        }
    }

    /// Records the chosen row's tag. **This is the callback.**
    pub fn record(&self, tag: isize) {
        self.tag.set(tag);
    }

    /// The recorded tag, or `None` if nothing was chosen.
    #[must_use]
    pub fn taken(&self) -> Option<isize> {
        let tag = self.tag.get();
        (tag >= 0).then_some(tag)
    }
}

/// **Not derived.** A derived `Default` would zero the field, and zero is the
/// tag of the menu's *first row* — so a menu that was never opened would report
/// its first item as chosen. Written out so that the default is
/// [`Selection::NOTHING`] by construction.
impl Default for Selection {
    fn default() -> Self {
        Self::new()
    }
}

/// Why a menu was not shown.
///
/// Every variant means **nothing happened**: no window appeared, no `AppKit`
/// object was created, and no callback will fire later.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[non_exhaustive]
pub enum MenuError {
    /// Asked for from a thread that is not the main thread.
    ///
    /// `NSMenu` is a `MainThreadOnly` class; this is the dynamic half of the
    /// rule the `MainThreadMarker` enforces statically.
    OffMainThread,
    /// This process is already showing a menu.
    ///
    /// Two nested tracking loops is not a state `AppKit` is documented to survive,
    /// and the second menu would in any case be the one the first one's dismissal
    /// closes.
    AlreadyTracking,
    /// This platform has no native context menu in this port.
    Unsupported,
}

impl fmt::Display for MenuError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let reason = match self {
            Self::OffMainThread => {
                "a native context menu can only be shown from the main thread: NSMenu is a \
                 MainThreadOnly class"
            }
            Self::AlreadyTracking => {
                "a native context menu is already open in this process; close it before showing \
                 another"
            }
            Self::Unsupported => "this platform has no native context menu in this build",
        };
        f.write_str(reason)
    }
}

impl std::error::Error for MenuError {}

/// Closes the menu this process is currently tracking, if there is one.
///
/// Answers `true` if a menu was tracking and has been told to close, `false` if
/// none was. The tracking loop inside [`ContextMenu::show_at`] returns shortly
/// afterwards, with no selection.
///
/// # What this is for
///
/// Two things, and the second is why it exists at all. A menu should close when
/// the state it was opened over stops existing — a pane that closes underneath
/// it, a workspace that switches. And it is what makes the menu **observable on a
/// machine that cannot click**: scheduling this on the main queue while a menu is
/// up proves the popup really opened and really closed, in an automated run, with
/// no synthetic events. See `crowbar-app`'s `native-menu` surface and its
/// `--dismiss-after`.
///
/// The main dispatch queue is drained in `NSEventTrackingRunLoopMode`, so a block
/// scheduled onto it *does* run while a menu is tracking. That is what makes
/// calling this from a scheduled task work rather than deadlock.
///
/// # Errors
///
/// [`MenuError::OffMainThread`] off the main thread; [`MenuError::Unsupported`]
/// on a platform with no implementation.
pub fn cancel_tracking() -> Result<bool, MenuError> {
    #[cfg(target_os = "macos")]
    {
        appkit::cancel()
    }
    #[cfg(not(target_os = "macos"))]
    {
        Err(MenuError::Unsupported)
    }
}

#[cfg(test)]
mod tests {
    use super::{ContextMenu, MenuError, MenuItem, ScreenPoint, Selection, cancel_tracking};

    /// The live fixture this module was written against: the comment-actions
    /// menu from `web/src/features/git/components/review-thread-item.tsx`, which
    /// is `dropdown-menu`'s own fixture too.
    fn comment_menu() -> ContextMenu {
        ContextMenu::new()
            .item("edit", "Edit")
            .item("copy", "Copy as Markdown")
            .separator()
            .item("delete", "Delete")
    }

    /// The four shapes `dropdown-menu.tsx`'s primitive offers, built through the
    /// chaining constructors and read back as data.
    #[test]
    fn the_builder_spells_item_separator_disabled_and_checked() {
        let menu = ContextMenu::new()
            .item("edit", "Edit")
            .checked_item("wrap", "Soft Wrap", true)
            .disabled_item("delete", "Delete")
            .separator()
            .submenu("Copy as…", ContextMenu::new().item("md", "Markdown"));

        assert_eq!(
            menu.items,
            vec![
                MenuItem::Item {
                    id: "edit".to_owned(),
                    title: "Edit".to_owned(),
                    enabled: true,
                    checked: false,
                },
                MenuItem::Item {
                    id: "wrap".to_owned(),
                    title: "Soft Wrap".to_owned(),
                    enabled: true,
                    checked: true,
                },
                MenuItem::Item {
                    id: "delete".to_owned(),
                    title: "Delete".to_owned(),
                    enabled: false,
                    checked: false,
                },
                MenuItem::Separator,
                MenuItem::Submenu {
                    title: "Copy as…".to_owned(),
                    enabled: true,
                    items: vec![MenuItem::Item {
                        id: "md".to_owned(),
                        title: "Markdown".to_owned(),
                        enabled: true,
                        checked: false,
                    }],
                },
            ],
        );
    }

    /// Tags are indices into the menu's **shape**: depth-first over every item,
    /// separators skipped, submenu parents skipped, and — the part that matters —
    /// a disabled row still occupying its place.
    #[test]
    fn tags_number_the_shape_and_a_disabled_row_keeps_its_place() {
        let menu = ContextMenu::new()
            .item("edit", "Edit")
            .separator()
            .disabled_item("copy", "Copy as Markdown")
            .submenu(
                "Copy as…",
                ContextMenu::new()
                    .item("md", "Markdown")
                    .item("txt", "Text"),
            )
            .item("delete", "Delete");

        assert_eq!(menu.chosen_ids(), ["edit", "copy", "md", "txt", "delete"]);
        for (tag, id) in ["edit", "copy", "md", "txt", "delete"]
            .into_iter()
            .enumerate()
        {
            let tag = isize::try_from(tag).expect("five fits");
            assert_eq!(menu.id_for_tag(tag), Some(id));
        }

        // Enabling the disabled row must not renumber anything around it, which
        // is the whole reason disabled rows are tagged.
        let enabled = ContextMenu::new()
            .item("edit", "Edit")
            .separator()
            .item("copy", "Copy as Markdown")
            .submenu(
                "Copy as…",
                ContextMenu::new()
                    .item("md", "Markdown")
                    .item("txt", "Text"),
            )
            .item("delete", "Delete");
        assert_eq!(enabled.chosen_ids(), menu.chosen_ids());
    }

    /// A tag no row has is a miss, not a panic — including the negative one that
    /// Objective-C can hand back.
    #[test]
    fn a_tag_outside_the_menu_names_nothing() {
        let menu = comment_menu();

        assert_eq!(menu.id_for_tag(3), None);
        assert_eq!(menu.id_for_tag(-1), None);
        assert_eq!(menu.id_for_tag(isize::MIN), None);
        assert_eq!(menu.id_for_tag(isize::MAX), None);
        assert_eq!(ContextMenu::new().id_for_tag(0), None);
    }

    /// **The trap a derived `Default` would have set.** Zero is a valid tag, so a
    /// zeroed selection would report the menu's first row as chosen by a user who
    /// never opened it.
    #[test]
    fn a_fresh_selection_is_nothing_and_not_the_first_row() {
        for fresh in [Selection::new(), Selection::default()] {
            assert_eq!(fresh.taken(), None);
            assert_eq!(comment_menu().chosen(&fresh), None);
        }
    }

    /// **The selection path, driven the way `AppKit` drives it.** `record` is the
    /// action callback's whole body; calling it here is the same line, not a
    /// stand-in for it.
    #[test]
    fn recording_a_tag_resolves_to_that_rows_id() {
        let menu = comment_menu();

        for (tag, id) in [(0_isize, "edit"), (1, "copy"), (2, "delete")] {
            let selection = Selection::new();
            selection.record(tag);
            assert_eq!(selection.taken(), Some(tag));
            assert_eq!(menu.chosen(&selection), Some(id));
        }

        // Dismissal: AppKit sends no action at all, so the selection is
        // untouched and resolves to nothing.
        let dismissed = Selection::new();
        assert_eq!(menu.chosen(&dismissed), None);

        // A tag recorded against a menu that no longer has that row is a miss.
        let stale = Selection::new();
        stale.record(2);
        assert_eq!(ContextMenu::new().item("only", "Only").chosen(&stale), None);
    }

    /// **Main-thread enforcement, observed rather than asserted about.** Run on a
    /// thread that is provably not the main one — a spawned one, whatever
    /// `--test-threads` is — showing a menu is a refusal with nothing shown.
    #[test]
    fn a_menu_asked_for_off_the_main_thread_is_refused_not_shown() {
        let menu = comment_menu();
        let outcome = std::thread::spawn(move || {
            (
                menu.show_at(ScreenPoint::new(100.0, 100.0)).err(),
                cancel_tracking().err(),
            )
        })
        .join()
        .expect("the spawned thread did not panic");

        let expected = if cfg!(target_os = "macos") {
            MenuError::OffMainThread
        } else {
            MenuError::Unsupported
        };
        assert_eq!(outcome, (Some(expected), Some(expected)));
    }

    /// Every refusal says which one it is and what to do about it, because
    /// "could not show menu" is not something anyone can act on.
    #[test]
    fn every_refusal_explains_itself() {
        for (error, word) in [
            (MenuError::OffMainThread, "main thread"),
            (MenuError::AlreadyTracking, "already open"),
            (MenuError::Unsupported, "platform"),
        ] {
            let said = error.to_string();
            assert!(said.contains(word), "{said}");
        }
    }

    /// The y flip, which is the one piece of arithmetic between a GPUI point and
    /// an `AppKit` one — and the only place a caller can silently put a menu on the
    /// wrong half of the screen.
    #[test]
    fn the_top_left_conversion_reflects_y_and_leaves_x_alone() {
        let screen = 1080.0;

        assert_eq!(
            ScreenPoint::from_top_left(300.0, 0.0, screen),
            ScreenPoint::new(300.0, 1080.0),
            "the top of the display is the top in both spaces",
        );
        assert_eq!(
            ScreenPoint::from_top_left(300.0, 1080.0, screen),
            ScreenPoint::new(300.0, 0.0),
            "and so is the bottom",
        );
        assert_eq!(
            ScreenPoint::from_top_left(12.5, 540.0, screen),
            ScreenPoint::new(12.5, 540.0),
        );
        // It is its own inverse, which is what makes a round trip through a
        // window's origin harmless.
        let there = ScreenPoint::from_top_left(40.0, 700.0, screen);
        assert_eq!(
            ScreenPoint::from_top_left(there.x, there.y, screen),
            ScreenPoint::new(40.0, 700.0),
        );
    }

    /// An empty menu is a picture the platform has — a small empty popup — so it
    /// is not refused before it is shown.
    #[test]
    fn an_empty_menu_is_a_menu() {
        let empty = ContextMenu::new();

        assert!(empty.items.is_empty());
        assert!(empty.chosen_ids().is_empty());
        assert_eq!(empty, ContextMenu::default());
    }
}
