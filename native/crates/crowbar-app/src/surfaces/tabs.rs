//! `--surface tabs` — a tab list, its tabs and the indicator under the active
//! one.
//!
//! The fixture is `web/src/components/layout/sidebar-tab-bar.tsx`, which is one
//! of the app's two `Tabs` and the only one whose root contains **its own
//! anchors and nothing else** — see [`SURFACE`] and
//! [`crowbar_ui::primitives::tabs::Tabs::fixture`].
//!
//! # The state axis, and the one flag that is real
//!
//! | flag | here |
//! |---|---|
//! | `selected` | **the only real one**, and it moves an anchored box rather than tinting one: the indicator is *under the active tab*, so selecting `--active` moves `tab-indicator`'s `x` by a whole tab and flips `data-active:text-foreground` on two tabs. Without it the fixture rests on its first tab, which is where `useSidebarStore`'s `getInitialState()` leaves it. |
//! | `hover` | a **real** state, and what it paints depends on the variant. Under `default` the only rule is `hover:text-muted-foreground`, which colours a label that is the call site's `<span>` — unanchored, and `display: none` in the live cell. Under `underline` the list adds `*:data-[slot=tabs-tab]:hover:bg-accent`, which *is* an anchored box's `bg` — but `underline` has no live call site, so the cell that could fail has no reference. |
//! | `focus` | a **real** state with **no field**. `focus-visible:ring-2` is a box-shadow and `ANCHORS.md` §6 has none for shadows; `outline-none` is an outline, which it has none for either. |
//! | `empty` | **unmodelled.** A `Tabs` with no tabs is not a state the app has: `sidebar-tab-bar.tsx` renders at least three and `git-panel.tsx` two, both unconditionally. |
//! | `loading`, `error` | unmodelled, as on every surface so far. |
//!
//! `hover` and `focus` are **not** declared unmodelled: `Surface::unmodelled`
//! means "this surface's React original has no such state", and both originals
//! exist. What is missing is a field in the contract, or a reference — two
//! different facts, and both belong in the per-cell caption where
//! [`Params::describe`] puts them.
//!
//! # Why the reference is the sidebar's tab bar and not the git panel's
//!
//! `ANCHORS.md` v1.8: a snapshot carries exactly the surface's own anchors. The
//! React extractor walks every `data-oracle-id` under the root, and this root's
//! subtree is the list, the tabs and the indicator — a tab's children are one
//! phosphor SVG and one `<span>`, neither of which carries an anchor. The git
//! panel's `Tabs` root contains `ChangedFilesTree`, whose rows carry
//! `git-row-icon`, `git-row-name`, `git-row-badge` and the rest, so a capture
//! there swallows `git-status-row` **whenever the fixture workspace has a changed
//! file**. Measured on the live app with a clean tree: 0 foreign anchors. That is
//! the point — it is a property of the *cell*, which is why `tabs` declares no
//! scope in `oracleSurfaceScope` and why the honest reference is the root that
//! cannot acquire one.
//!
//! # Three options that are not matrix axes
//!
//! `--tabs`, `--list-bg` and `--tab-sizing` are the call site's, not the
//! primitive's: the values decide the anchor **set**, and the other two are what
//! `tailwind-merge` leaves after `sidebar-tab-bar.tsx`'s `className`. They are
//! runtime inputs in exactly the sense `resizable`'s `--grow` is.

use std::fmt::Write as _;

use crowbar_ui::IconName;
use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::tabs::{
    self, ListBackground, Orientation, Panel, Tab, TabSizing, Tabs, Variant,
};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "tabs",
    root: tabs::ID_ROOT,
    unmodelled: &[StateFlag::Empty, StateFlag::Loading, StateFlag::Error],
    // A tab bar is one line: `h-9` inside a `p-0.5` list is 40px, and the rest
    // of the floor is the caption and the top inset. The two Phase 1 rows' 200
    // is the smallest number any surface here uses and this is the same shape of
    // thing, so it takes the same one rather than inventing a tighter one.
    min_window_height: 200,
    // **The reference does not fill its window.** `sidebar-tab-bar.tsx` draws
    // inside the sidebar: a 278px `tabs` root measured live at `innerWidth`
    // 1714. So the surface width and the viewport width are two quantities here,
    // exactly as they are for `sidebar-carousel`, and the horizontal inset stays.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// `--active`'s default: **`files`**, the third tab.
///
/// Chosen the way `sidebar-carousel`'s `--active-tab files` was — so that a bare
/// `--flags selected` renders something that can fail. The resting cell puts the
/// indicator on `workspaces` at `x: 2`; this one puts it two tabs along, which
/// moves `tab-indicator`'s `x` by 184px and swaps `data-active` between two tabs.
/// Defaulting it to the first tab would make the flag a no-op and every
/// `selected` cell a repeat of the resting one.
pub const DEFAULT_ACTIVE: &str = "files";

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--orientation`.
    pub orientation: Orientation,
    /// `--variant`.
    pub variant: Variant,
    /// `--list-bg`.
    pub list_background: ListBackground,
    /// `--tab-sizing`.
    pub tab_sizing: TabSizing,
    /// `--tabs`: the tab values in DOM order.
    ///
    /// **The anchor set.** Each value becomes `tabs-tab-<value>`, so this list is
    /// what a reference capture has to agree with, name for name.
    pub tabs: Vec<String>,
    /// `--active`: which tab `--flags selected` activates.
    pub active: String,
    /// `--panel`: the value of the one mounted `TabsPanel`, if any.
    pub panel: Option<String>,
    /// `--labels`: render the call site's label spans.
    pub labels: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Tabs::fixture();
        Self {
            orientation: fixture.orientation,
            variant: fixture.variant,
            list_background: fixture.list_background,
            tab_sizing: fixture.tab_sizing,
            tabs: fixture
                .tabs
                .iter()
                .map(|tab| tab.value.to_string())
                .collect(),
            active: DEFAULT_ACTIVE.to_owned(),
            panel: None,
            labels: false,
        }
    }
}

impl Params {
    /// The tabs this cell describes.
    ///
    /// Built by taking the live fixture and applying the cell, rather than by
    /// assembling one from scratch, so that a bare `--surface tabs` renders the
    /// tab bar the reference actually has.
    #[must_use]
    pub fn tabs(&self, cell: &Cell) -> Tabs {
        // Which tab carries `data-active`. `--flags selected` moves it to
        // `--active`; without the flag the fixture rests on its first tab, which
        // is what `getInitialState()` leaves the sidebar store on.
        let active: &str = if cell.has(StateFlag::Selected) {
            &self.active
        } else {
            self.tabs.first().map_or("", String::as_str)
        };

        let tabs = self
            .tabs
            .iter()
            .map(|value| {
                let mut tab = Tab::new(value.clone());
                tab.selected = value == active;
                tab.label = self.labels.then(|| value.clone().into());
                // The same glyph the sidebar's own tab bar draws, so this
                // cell measures the tab the app renders rather than an
                // iconless one — `sidebar_tab_bar::glyph`'s mapping.
                tab.icon = Some(match value.as_str() {
                    "chats" => IconName::ChatsCircle,
                    "files" => IconName::FolderOpen,
                    "git" => IconName::GitBranch,
                    _ => IconName::SquaresFour,
                });
                tab
            })
            .collect();

        Tabs {
            orientation: self.orientation,
            variant: self.variant,
            list_background: self.list_background,
            tab_sizing: self.tab_sizing,
            // The `sm:` variants are a **viewport** media query, so they follow
            // the viewport rather than `--width` — the same quantity
            // `git-status-row` reads, and conflating the two is how a reference
            // captured below 640px came to be compared against the `sm:` arm.
            breakpoint: cell.breakpoint(),
            tabs,
            panel: self.panel.as_ref().map(Panel::new),
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
            "--orientation" => self.orientation = parse_orientation(&value(args, option)?)?,
            "--variant" => self.variant = parse_variant(&value(args, option)?)?,
            "--list-bg" => self.list_background = parse_list_background(&value(args, option)?)?,
            "--tab-sizing" => self.tab_sizing = parse_tab_sizing(&value(args, option)?)?,
            "--tabs" => self.tabs = parse_tabs(&value(args, option)?)?,
            "--active" => self.active = parse_value(&value(args, option)?, option)?,
            "--panel" => self.panel = Some(parse_value(&value(args, option)?, option)?),
            "--labels" => self.labels = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** Nothing on this command line sets a height: the list's is its
    /// tabs' `h-9`/`sm:h-8` plus `p-0.5`, and a panel's is whatever is left of
    /// the box the surface is drawn in. So this surface keeps
    /// [`Surface::min_window_height`], which is what a row and a menu popup do
    /// and for the same reason.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    /// What this cell renders, and — where it applies — that an axis was moved
    /// this surface cannot paint or cannot be compared on.
    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(
            out,
            " · {} · {} · tabs {} · list {} · {}",
            self.variant.name(),
            self.orientation.name(),
            self.tabs.join("/"),
            self.list_background.name(),
            self.tab_sizing.name(),
        );
        if let Some(panel) = &self.panel {
            let _ = write!(out, " · panel {panel}");
        }
        if self.labels {
            out.push_str(" · labels");
        }

        // The active tab is the picture, so it is named on every cell rather
        // than only on the ones the flag moved.
        let active = if cell.has(StateFlag::Selected) {
            self.active.clone()
        } else {
            self.tabs.first().cloned().unwrap_or_default()
        };
        let _ = write!(out, " · active {active}");
        if cell.has(StateFlag::Selected) && Some(&self.active) == self.tabs.first() {
            out.push_str(
                " · selected: --active is already the resting tab, so this cell is the \
                 resting one and cannot fail",
            );
        }

        if !self.labels {
            out.push_str(
                " · no label is rendered, so nothing here paints text and --content \
                 cannot fail",
            );
        }
        if cell.has(StateFlag::Hover) {
            out.push_str(match self.variant {
                Variant::Default => {
                    " · hover: the only hover rule colours the call site's label span, \
                     which carries no anchor, so this cell cannot fail"
                }
                Variant::Underline => {
                    " · hover: underline's hover:bg-accent does paint a tab's background, \
                     but no live call site asks for underline, so there is no reference"
                }
            });
        }
        if cell.has(StateFlag::Focus) {
            out.push_str(
                " · focus: the ring is a box-shadow, which ANCHORS.md §6 has no field \
                 for, so this cell cannot fail",
            );
        }
        if self.variant == Variant::Underline {
            out.push_str(" · underline: no live call site asks for it, so there is no reference");
        }
        if self.orientation == Orientation::Vertical {
            out.push_str(" · vertical: no live call site asks for it, so there is no reference");
        }
        if self.tab_sizing == TabSizing::Intrinsic {
            out.push_str(
                " · intrinsic: tailwind-merge replaces shrink-0 grow with flex-1 at every \
                 live call site, so there is no reference",
            );
        }
        if self.panel.is_some() {
            out.push_str(
                " · panel: the only live TabsPanel is git-panel.tsx's, whose Tabs root \
                 also holds git-row-* anchors, so there is no clean reference",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.tabs(cell).render(theme, anchors)
    }
}

fn parse_orientation(raw: &str) -> Result<Orientation, ParseError> {
    match raw {
        "horizontal" => Ok(Orientation::Horizontal),
        "vertical" => Ok(Orientation::Vertical),
        other => Err(ParseError::Rejected(format!(
            "--orientation takes horizontal or vertical, not {other}"
        ))),
    }
}

fn parse_variant(raw: &str) -> Result<Variant, ParseError> {
    match raw {
        "default" => Ok(Variant::Default),
        "underline" => Ok(Variant::Underline),
        other => Err(ParseError::Rejected(format!(
            "--variant takes default or underline, not {other}"
        ))),
    }
}

fn parse_list_background(raw: &str) -> Result<ListBackground, ParseError> {
    match raw {
        "muted" => Ok(ListBackground::Muted),
        "sidebar-element-idle" => Ok(ListBackground::SidebarElementIdle),
        other => Err(ParseError::Rejected(format!(
            "--list-bg takes muted or sidebar-element-idle, not {other}"
        ))),
    }
}

fn parse_tab_sizing(raw: &str) -> Result<TabSizing, ParseError> {
    match raw {
        "fill" => Ok(TabSizing::Fill),
        "intrinsic" => Ok(TabSizing::Intrinsic),
        other => Err(ParseError::Rejected(format!(
            "--tab-sizing takes fill or intrinsic, not {other}"
        ))),
    }
}

/// One tab value: non-empty, and made only of the characters an anchor id can
/// carry.
///
/// Checked rather than trusted, because the value **becomes an anchor id**:
/// `tabs-tab-<value>`. A value with a space in it produces an id no `--report`
/// line can be read against, and an empty one produces the bare prefix — which
/// two empty values would then share, and `ANCHORS.md` v1.8 makes a repeated id a
/// refusal rather than a delta.
fn parse_value(raw: &str, option: &str) -> Result<String, ParseError> {
    if raw.is_empty()
        || !raw
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
    {
        return Err(ParseError::Rejected(format!(
            "{option} takes a non-empty value of [A-Za-z0-9_-], because it becomes the \
             anchor id tabs-tab-<value>; got {raw:?}"
        )));
    }
    Ok(raw.to_owned())
}

/// The tab values, in DOM order: at least one, all distinct.
///
/// Distinctness is the anchor contract rather than a nicety — two tabs sharing a
/// value share an anchor id, and v1.8 refuses a snapshot carrying one twice
/// instead of picking whichever arrived last.
fn parse_tabs(raw: &str) -> Result<Vec<String>, ParseError> {
    let mut out = Vec::new();
    for word in raw.split(',') {
        out.push(parse_value(word.trim(), "--tabs")?);
    }
    let mut sorted = out.clone();
    sorted.sort();
    let before = sorted.len();
    sorted.dedup();
    if sorted.len() != before {
        return Err(ParseError::Rejected(format!(
            "--tabs takes distinct values: each one becomes the anchor id \
             tabs-tab-<value>, and ANCHORS.md v1.8 refuses a snapshot carrying one \
             anchor twice rather than choosing between them; got {raw}"
        )));
    }
    Ok(out)
}

fn options() -> Vec<(String, String)> {
    let fixture = Params::default();
    [
        (
            "--variant <name>".to_owned(),
            format!(
                "default|underline; only default has a reference [{}]",
                fixture.variant.name(),
            ),
        ),
        (
            "--orientation <name>".to_owned(),
            format!(
                "horizontal|vertical; only horizontal has a reference [{}]",
                fixture.orientation.name(),
            ),
        ),
        (
            "--tabs <a,b,c>".to_owned(),
            format!(
                "the tab values in DOM order; each becomes tabs-tab-<value> [{}]",
                fixture.tabs.join(","),
            ),
        ),
        (
            "--active <value>".to_owned(),
            format!("which tab --flags selected activates [{DEFAULT_ACTIVE}]"),
        ),
        (
            "--list-bg <name>".to_owned(),
            format!(
                "muted|sidebar-element-idle; the call site's override [{}]",
                fixture.list_background.name(),
            ),
        ),
        (
            "--tab-sizing <name>".to_owned(),
            format!(
                "fill|intrinsic; intrinsic is the primitive's own and unreferenced [{}]",
                fixture.tab_sizing.name(),
            ),
        ),
        (
            "--panel <value>".to_owned(),
            "mount one TabsPanel as tabs-content-<value> [none]".to_owned(),
        ),
        (
            "--labels".to_owned(),
            "render the call site's label spans; unanchored either way [off]".to_owned(),
        ),
    ]
    .into_iter()
    .collect()
}

#[cfg(test)]
mod tests {
    use super::{DEFAULT_ACTIVE, Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};
    use crowbar_ui::primitives::tabs::{
        ID_INDICATOR, ID_LIST, ID_ROOT, ListBackground, Orientation, TabSizing, Tabs, Variant,
    };

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "tabs"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a tabs cell carries this surface's bag")
    }

    fn built(cell: &Cell) -> Tabs {
        params_of(cell).tabs(cell)
    }

    /// The active tab's value, or `None` where nothing is active.
    fn active(cell: &Cell) -> Option<String> {
        built(cell).active().map(|tab| tab.value.to_string())
    }

    #[test]
    fn the_defaults_are_the_live_sidebar_tab_bar() {
        let bag = Params::default();
        let fixture = Tabs::fixture();

        assert_eq!(bag.orientation, fixture.orientation);
        assert_eq!(bag.variant, fixture.variant);
        assert_eq!(bag.list_background, fixture.list_background);
        assert_eq!(bag.tab_sizing, fixture.tab_sizing);
        assert_eq!(bag.tabs, ["workspaces", "chats", "files"]);
        assert!(bag.panel.is_none());
        assert!(!bag.labels);

        // And the bag rebuilds the fixture, rather than something that merely
        // resembles it.
        assert_eq!(built(&cell(&[])), fixture);
    }

    /// **`selected` is the only flag that moves an anchored box**, and it moves
    /// `tab-indicator` by a whole tab rather than tinting one.
    #[test]
    fn selected_moves_the_active_tab_and_nothing_else_does() {
        assert_eq!(active(&cell(&[])).as_deref(), Some("workspaces"));
        assert_eq!(
            active(&cell(&["--flags", "selected"])).as_deref(),
            Some(DEFAULT_ACTIVE),
        );
        assert_ne!(DEFAULT_ACTIVE, "workspaces");

        // Driven explicitly, the flag reaches whichever tab was named.
        assert_eq!(
            active(&cell(&["--flags", "selected", "--active", "chats"])).as_deref(),
            Some("chats"),
        );
        // `--active` without the flag changes nothing: the flag is what applies
        // it, exactly as `sidebar-carousel`'s `--active-tab` needs `selected`.
        assert_eq!(
            active(&cell(&["--active", "chats"])).as_deref(),
            Some("workspaces"),
        );

        // The other two real flags reach nothing this surface can paint into an
        // anchor, so the picture is the resting one.
        for flags in ["hover", "focus", "hover,focus"] {
            assert_eq!(
                built(&cell(&["--flags", flags])),
                Tabs::fixture(),
                "{flags}"
            );
        }

        // Exactly one tab is ever active.
        for line in [vec![], vec!["--flags", "selected"]] {
            let driven = built(&cell(&line));
            assert_eq!(driven.tabs.iter().filter(|tab| tab.selected).count(), 1);
        }
    }

    /// **An `--active` the tab list does not contain leaves nothing active**,
    /// which is a picture base-ui can produce — `value` with no matching tab
    /// renders no indicator at all — rather than a silent fallback to the first
    /// tab.
    #[test]
    fn an_unknown_active_value_selects_nothing() {
        let orphan = cell(&["--flags", "selected", "--active", "missing"]);
        assert!(active(&orphan).is_none());
        assert!(built(&orphan).tabs.iter().all(|tab| !tab.selected));
    }

    /// `--tabs` decides the **anchor set**, name for name, and the ids are what
    /// the reference capture has to agree with.
    #[test]
    fn the_tab_values_are_the_anchor_ids() {
        let driven = built(&cell(&["--tabs", "changes,history"]));

        let ids: Vec<String> = driven
            .tabs
            .iter()
            .map(|tab| tab.anchor().to_string())
            .collect();
        assert_eq!(ids, ["tabs-tab-changes", "tabs-tab-history"]);

        // The panel's too, and it is off unless asked for.
        assert!(built(&cell(&[])).panel.is_none());
        let panelled = built(&cell(&["--panel", "changes"]));
        assert_eq!(
            panelled
                .panel
                .as_ref()
                .map(|panel| panel.anchor().to_string()),
            Some("tabs-content-changes".to_owned()),
        );
    }

    /// The three call-site parameters reach the component, and the two that a
    /// live call site does **not** produce say so in the caption.
    #[test]
    fn the_call_site_parameters_reach_the_component() {
        let driven = built(&cell(&[
            "--list-bg",
            "muted",
            "--tab-sizing",
            "intrinsic",
            "--variant",
            "underline",
            "--orientation",
            "vertical",
        ]));
        assert_eq!(driven.list_background, ListBackground::Muted);
        assert_eq!(driven.tab_sizing, TabSizing::Intrinsic);
        assert_eq!(driven.variant, Variant::Underline);
        assert_eq!(driven.orientation, Orientation::Vertical);

        for (line, needle) in [
            (
                vec!["--variant", "underline"],
                "underline: no live call site",
            ),
            (
                vec!["--orientation", "vertical"],
                "vertical: no live call site",
            ),
            (
                vec!["--tab-sizing", "intrinsic"],
                "intrinsic: tailwind-merge",
            ),
            (vec!["--panel", "changes"], "panel: the only live TabsPanel"),
        ] {
            let described = cell(&line).describe();
            assert!(described.contains(needle), "{described}");
            assert!(described.contains("no reference") || described.contains("no clean reference"));
        }

        // `--list-bg muted` is the primitive's own and `git-panel.tsx` renders
        // it, so it is *not* declared unreferenced.
        let muted = cell(&["--list-bg", "muted"]).describe();
        assert!(muted.contains("list muted"), "{muted}");
        assert!(!muted.contains("no reference"), "{muted}");
    }

    /// **The two invisible states are named in the caption**, and `hover` says
    /// something different under each variant because it paints something
    /// different.
    #[test]
    fn the_two_invisible_states_are_named_per_variant() {
        let hover = cell(&["--flags", "hover"]).describe();
        assert!(hover.contains("label span"), "{hover}");
        assert!(hover.contains("cannot fail"), "{hover}");

        let underline_hover = cell(&["--flags", "hover", "--variant", "underline"]).describe();
        assert!(
            underline_hover.contains("hover:bg-accent"),
            "{underline_hover}"
        );
        assert!(
            underline_hover.contains("no reference"),
            "{underline_hover}"
        );

        let focus = cell(&["--flags", "focus"]).describe();
        assert!(focus.contains("box-shadow"), "{focus}");
        assert!(focus.contains("cannot fail"), "{focus}");

        // The originals exist, so neither is declared unmodelled.
        assert!(!SURFACE.unmodelled(StateFlag::Hover));
        assert!(!SURFACE.unmodelled(StateFlag::Focus));
        assert!(!SURFACE.unmodelled(StateFlag::Selected));
        for flag in [StateFlag::Empty, StateFlag::Loading, StateFlag::Error] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }

    /// The content axis is vacuous unless `--labels` puts a string on screen,
    /// and the caption says so on every cell where it is.
    #[test]
    fn the_content_axis_is_vacuous_without_labels() {
        for content in ["short", "normal", "overflow"] {
            let described = cell(&["--content", content]).describe();
            assert!(described.contains("--content cannot fail"), "{described}");
        }
        let labelled = cell(&["--labels"]).describe();
        assert!(!labelled.contains("--content cannot fail"), "{labelled}");
        assert!(labelled.contains("labels"), "{labelled}");
        assert!(
            built(&cell(&["--labels"]))
                .tabs
                .iter()
                .all(|tab| tab.label.is_some())
        );
    }

    /// A `--flags selected` that lands on the resting tab is the resting cell,
    /// and the caption says so rather than letting a reader bank it.
    #[test]
    fn a_selected_cell_that_did_not_move_says_so() {
        let stuck = cell(&["--flags", "selected", "--active", "workspaces"]).describe();
        assert!(stuck.contains("cannot fail"), "{stuck}");
        assert!(stuck.contains("resting tab"), "{stuck}");

        let moved = cell(&["--flags", "selected"]).describe();
        assert!(!moved.contains("resting tab"), "{moved}");
    }

    /// The vocabulary is closed, and **a value that could not be an anchor id is
    /// a rejection** rather than an id nobody can read a report against.
    #[test]
    fn the_option_vocabulary_is_closed() {
        for line in [
            vec!["--variant", "pill"],
            vec!["--variant"],
            vec!["--orientation", "sideways"],
            vec!["--orientation"],
            vec!["--list-bg", "red"],
            vec!["--list-bg"],
            vec!["--tab-sizing", "auto"],
            vec!["--tab-sizing"],
            vec!["--tabs", ""],
            vec!["--tabs", "a,,b"],
            vec!["--tabs", "one two"],
            vec!["--tabs", "a,b,a"],
            vec!["--tabs"],
            vec!["--active", ""],
            vec!["--active", "a b"],
            vec!["--active"],
            vec!["--panel", "a b"],
            vec!["--panel"],
        ] {
            let mut full = vec!["--surface", "tabs"];
            full.extend_from_slice(&line);
            assert!(
                matches!(
                    Cell::parse(full.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }

        // The repeated value names the rule it breaks, because "invalid" is not
        // something anyone can act on.
        let Err(ParseError::Rejected(complaint)) = Cell::parse(
            ["--surface", "tabs", "--tabs", "a,b,a"]
                .iter()
                .map(|arg| (*arg).to_owned()),
        ) else {
            panic!("two tabs cannot share a value");
        };
        assert!(complaint.contains("v1.8"), "{complaint}");
    }

    /// **These options belong to this surface and to no other.**
    #[test]
    fn this_surfaces_options_are_rejected_on_another_surface() {
        for option in [
            "--variant",
            "--list-bg",
            "--tab-sizing",
            "--tabs",
            "--active",
        ] {
            let line = ["--surface", "dropdown-menu", option, "default"];
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{option} should not be a dropdown-menu option",
            );
        }
        // `--labels` and `--panel` too, and they are checked separately because
        // one takes no value.
        for line in [
            vec!["--surface", "dropdown-menu", "--labels"],
            vec!["--surface", "dropdown-menu", "--panel", "changes"],
        ] {
            assert!(
                matches!(
                    Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
                    Err(ParseError::Rejected(_)),
                ),
                "{line:?} should have been rejected",
            );
        }
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();

        for option in [
            "--variant",
            "--orientation",
            "--tabs",
            "--active",
            "--list-bg",
            "--tab-sizing",
            "--panel",
            "--labels",
        ] {
            assert!(usage.contains(option), "{option} is missing from the usage");
        }
        assert!(usage.contains("tabs"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's contract fields, which a snapshot carries verbatim.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "tabs");
        assert_eq!(SURFACE.root, "tabs");
        assert_eq!(SURFACE.root, ID_ROOT);
        assert!(!SURFACE.full_bleed);

        // The three fixed ids the primitive writes, all distinct from each other
        // and from the root.
        let ids = [ID_ROOT, ID_LIST, ID_INDICATOR];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
    }
}
