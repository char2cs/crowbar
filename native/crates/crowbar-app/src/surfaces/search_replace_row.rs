//! `--surface search-replace-row` — `SearchReplaceRow`, standalone (P3.37).
//!
//! # Why this is a second surface and not a flag on `search`
//!
//! `search.rs`'s own module docs already anticipated the need: "reproducing
//! `/tmp/p3-ref-search-replace-row.json` needs a **second, scoped** capture …
//! not a flag on this one." This surface is that capture, made real rather
//! than left as a footnote — P3.37 found the reference refused (`root:
//! "search-replace-row"` names no registered surface) and had to decide
//! between two repairs: fold the row's anchors into `search`'s own declared
//! scope, or register it. The first is not available:
//!
//! * `search.tsx`'s `input.tsx` hard-codes `data-oracle-id="input-control"`/
//!   `"input"` on **every** `<Input>`, with no override prop. Expanding the
//!   replace row mounts a *second* one while `SearchPopover`'s own is still
//!   mounted, so a document spanning both carries the id twice —
//!   `native/oracle/ANCHORS.md` v1.8 refuses that outright, and no scope
//!   declaration changes it: `web/src/lib/oracle/extract.ts`'s
//!   `oracleSelectDeclaredAnchors` throws on a *declared* anchor found twice,
//!   it does not pick one.
//! * The driver side has the identical shape from the other direction:
//!   `AnchorRegistry::record` **replaces** rather than refuses a repeated id,
//!   so `--surface search --replace` does not crash — but its recorded
//!   `"input-control"` silently becomes whichever `Input` prepainted last
//!   (this row's own, narrower one), and `Snapshot::build` copies *every*
//!   recorded anchor into a snapshot regardless of which id `root` names (it
//!   re-origins, it does not filter by subtree) — so asking that same run for
//!   `root: "search-replace-row"` would still carry `search-popover` and
//!   everything in it, not the six-anchor shape the reference has.
//!
//! So the only render pass that can produce a registry shaped like
//! `/tmp/p3-ref-search-replace-row.json` is one that paints **nothing but**
//! `SearchReplaceRow` — which is what this surface is. `search`'s own
//! `--replace` flag is untouched: it still exists, still mounts the row
//! nested (for the `preserve-case` toggle and the row's own visual
//! composition to be exercised together), and is still the flag
//! `crowbar_ui::surfaces::search::SearchReplaceRow::render`'s module docs
//! refer to — this surface reuses the identical
//! [`crowbar_ui::surfaces::search::SearchReplaceRow`] struct and its
//! `render_root` method (the `AnchorSink::root` sibling of `render`,
//! added by this item), not a second implementation.
//!
//! # The state axis
//!
//! `Empty` is **real**, and drives both `can_replace` and `can_replace_all`
//! to `false` together — "the search that feeds this row found nothing, so
//! neither action has anything to do," the same reading `search`'s own
//! default cell already gives an empty query (`can_navigate: false`, module
//! docs). It is the one flag every other structural, mostly-static surface
//! in this tree also lands on when nothing else fits —
//! `sidebar_header`/`scroll_area`/`sidebar_empty` all declare exactly the
//! same five flags unmodelled and leave `Empty` real, each for its own
//! domain's version of "nothing here" — and `surface::tests::
//! no_surface_declares_its_entire_state_axis_unmodelled` is the guard that
//! rule serves: a surface with no real axis at all is one whose whole matrix
//! cannot fail. The other five are unmodelled for `search`'s own reasons,
//! restated per flag below.

use crowbar_ui::Theme;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::search::{self, SearchReplaceRow};
use gpui::AnyElement;

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "search-replace-row",
    root: search::ID_REPLACE_ROW,
    unmodelled: &[
        // `Loading`/`Error`: no original on any surface in this tree.
        StateFlag::Loading,
        StateFlag::Error,
        // `Hover`/`Focus`: real CSS on `searchActionButtonVariants`
        // (`hover:border-border/70 …`), unmodelled for `search`'s own
        // reason — no reference (`CGPreflightPostEventAccess()` false).
        StateFlag::Hover,
        StateFlag::Focus,
        // `Selected`: no toggle-shaped element on this row — `search`'s own
        // `Selected` drives the "Match case" toggle, which has no analogue
        // here.
        StateFlag::Selected,
    ],
    // The captured cell's own height (39px) plus the caption — a floor, not
    // a ceiling, matching `search`'s own reasoning: this surface drives no
    // window height of its own.
    min_window_height: 120,
    // Nested inside a 320px search popover in the reachable UI — never the
    // viewport — exactly `search`'s own declaration.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
///
/// `Default` is derived from [`SearchReplaceRow::fixture`]'s own state
/// (`can_replace`/`can_replace_all` both `true`) rather than restated, so a
/// change to that fixture moves this surface's default with it. The default
/// cell therefore does *not* match the one live captured cell's underlying
/// state exactly — an empty query really has both actions disabled — but
/// `search.md` §7 already records why the reference cannot tell the two
/// states apart: `bg` and `border` are `transparent` either way, so the
/// geometry is identical regardless, and [`StateFlag::Empty`] is what a cell
/// asks for the *other*, honestly-different one instead of pretending the
/// default already is it.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// Disables "Replace". Driven by [`StateFlag::Empty`] alone — see the
    /// module docs — not by an option of its own.
    pub can_replace: bool,
    /// Disables "All". Its own field rather than following `can_replace`,
    /// because the two legitimately diverge in `search.tsx` (`canReplaceAll`
    /// defaults to `canReplace` but is overridable) — [`SearchReplaceRow`]'s
    /// own struct keeps them apart for the identical reason, even though
    /// this surface's one state axis moves both together.
    pub can_replace_all: bool,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = SearchReplaceRow::fixture();
        Self {
            can_replace: fixture.can_replace,
            can_replace_all: fixture.can_replace_all,
        }
    }
}

impl Params {
    /// The row this cell describes. `StateFlag::Empty` overrides both fields
    /// to `false` together, regardless of what [`Default`] set them to —
    /// there is no CLI option that can put them back, because no reachable
    /// cell needs one (see the struct's own doc comment).
    #[must_use]
    pub fn row(&self, cell: &Cell) -> SearchReplaceRow {
        let empty = cell.has(StateFlag::Empty);
        SearchReplaceRow {
            can_replace: self.can_replace && !empty,
            can_replace_all: self.can_replace_all && !empty,
            breakpoint: cell.breakpoint(),
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        _option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        Ok(false)
    }

    /// **None.** The row authors its own height, exactly `search`'s own
    /// reasoning: no option here decides a window extent.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the search found nothing, so \"Replace\" and \"All\" are both \
                 disabled — geometrically identical to resting either way (bg/border stay \
                 transparent), see the module docs",
            );
        }
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.row(cell).render_root(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    Vec::new()
}

#[cfg(test)]
mod tests {
    use super::{Params, SURFACE, options};
    use crate::row_surface::{Cell, ParseError, StateFlag};

    fn cell(args: &[&str]) -> Cell {
        let mut line = vec!["--surface", "search-replace-row"];
        line.extend_from_slice(args);
        Cell::parse(line.iter().map(|arg| (*arg).to_owned())).expect("well-formed")
    }

    fn params_of(cell: &Cell) -> &Params {
        cell.surface_params::<Params>()
            .expect("a search-replace-row cell carries this surface's bag")
    }

    /// The default cell matches `SearchReplaceRow::fixture` — both actions
    /// enabled.
    #[test]
    fn the_default_cell_matches_the_fixture() {
        let bag = Params::default();
        assert!(bag.can_replace);
        assert!(bag.can_replace_all);
    }

    /// `--flags empty` disables both actions together — the one state axis
    /// this surface has, and the reason `Empty` is not in its own
    /// `unmodelled` list.
    #[test]
    fn empty_disables_both_actions_together() {
        let resting = cell(&[]);
        let row = params_of(&resting).row(&resting);
        assert!(row.can_replace);
        assert!(row.can_replace_all);

        let empty = cell(&["--flags", "empty"]);
        let row = params_of(&empty).row(&empty);
        assert!(!row.can_replace);
        assert!(!row.can_replace_all);
    }

    /// `Empty` is real; the other five are declared unmodelled — mutation
    /// evidence for `no_surface_declares_its_entire_state_axis_unmodelled`
    /// (`crate::surface::tests`): declaring `Empty` unmodelled too, as an
    /// earlier draft of this surface did with all six, is exactly what that
    /// guard exists to catch, and did catch.
    #[test]
    fn only_empty_is_modelled() {
        assert!(!SURFACE.unmodelled(StateFlag::Empty));
        for flag in [
            StateFlag::Loading,
            StateFlag::Error,
            StateFlag::Hover,
            StateFlag::Focus,
            StateFlag::Selected,
        ] {
            assert!(SURFACE.unmodelled(flag), "{flag:?}");
        }
    }

    /// The vocabulary is closed — this surface takes no options of its own
    /// at all, `--replace` included: that flag belongs to `search`.
    #[test]
    fn the_option_vocabulary_is_closed() {
        let line = ["--surface", "search-replace-row", "--replace"];
        assert!(matches!(
            Cell::parse(line.iter().map(|arg| (*arg).to_owned())),
            Err(ParseError::Rejected(_)),
        ));
    }

    /// Every option the parser accepts appears in the usage.
    #[test]
    fn the_usage_names_every_option_this_surface_takes() {
        let usage = crate::row_surface::usage();
        assert!(usage.contains("search-replace-row"));
        for (option, _) in options() {
            assert!(usage.contains(&option), "{option}");
        }
    }

    /// The registry entry's two contract fields, which a snapshot carries
    /// verbatim, and which are the whole reason this surface exists: a
    /// `--surface` whose own `root` really is `search-replace-row`.
    #[test]
    fn the_surface_names_itself_and_its_root_anchor() {
        assert_eq!(SURFACE.name, "search-replace-row");
        assert_eq!(SURFACE.root, "search-replace-row");
    }
}
