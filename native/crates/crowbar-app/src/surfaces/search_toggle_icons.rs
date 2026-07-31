//! `--surface search-toggle-icons` — four icons behind one anchor id.
//!
//! The default cell is the editor find bar's **Preserve case** toggle, measured
//! live at a 1714px viewport with the replace row expanded: `14.48 × 15`,
//! `fg #a4a4a4ff`, `CalSansUI` 11/15.71 at weight 600, `content_sized` and
//! `line_sized`. A glyph cell is archived beside it. See
//! `native/mapping/search-toggle-icons.md`,
//! `/tmp/p3-ref-search-toggle-icons.json` and
//! `/tmp/p3-ref-search-toggle-icons-glyph.json`.
//!
//! # One root, four renderings — and `--toggle` picks which
//!
//! All four entries carry `data-oracle-id="search-toggle-icon"`, so a reference
//! is taken with `extractSnapshot`'s `index` rather than by giving them four
//! ids. `Surface::root` is one string, and four ids would name a set no single
//! root contains: `ANCHORS.md` v1.8's rule, and `kbd`'s arrangement exactly.
//!
//! # Two shapes, and the contract sees very different amounts of them
//!
//! | Shape | What the contract can compare |
//! |---|---|
//! | the three phosphor glyphs | `bounds`, `bg`, `visible`, `radius`, `border.w` — **and nothing else.** An `<svg>` has no text nodes, so no `fg` and no `font`; `fill: currentColor` has no field |
//! | `preserve-case` | all of the above **plus** the full text group: `text`, `text_width`, `clipped`, `font` and `fg` |
//!
//! That asymmetry is why the *run-shaped* cell is the fixture even though three
//! of the four live icons are glyphs — `kbd`'s reason, where the `Esc` cap was
//! chosen over the three icon caps.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--toggle` | **real**, and it changes the anchor's *shape*: a glyph cell carries no text group at all. `ANCHORS.md` ranks a missing field group above a wrong number |
//! | `--viewport-width` | **real on both shapes.** The glyph's box is `button`'s `[&_svg:not([class*='size-'])]:size-4.5 sm:…size-4`, 18 → 16; and the span's inherited line-height ratio is `text-base`'s 1.5 → `sm:text-sm`'s 1.4285714, so its box floors at 16 or 15 |
//! | `--theme` | **real on `preserve-case` only.** `--muted-foreground` differs in the two tables and reaches the contract through `fg`; on a glyph the same colour reaches it through nothing |
//! | `--content` | **vacuous.** The legend is hard-coded `Aa` in the primitive; no call site supplies it |
//! | `--width` | **vacuous.** Neither shape stretches |
//!
//! # The state axis
//!
//! `hover` and `focus` are unmodelled: `search.tsx` puts both on the **button**
//! (`hover:border-border/70 hover:bg-muted`), which is a different anchor on a
//! different surface, and nothing on the icon itself responds.
//!
//! `selected` is **real, and only on `preserve-case`** — the recurring shape
//! `input` recorded for `error`. `searchToggleButtonVariants({ active: true })`
//! moves the button's `color` from `--muted-foreground` to `--foreground`, the
//! run inherits it, and `fg` is a compared field. Measured live by toggling the
//! control: `oklch(0.72 0 0)` → `oklch(0.97 0 0)`. On the three glyph cells the
//! same move is **invisible**, because `fill: currentColor` has no field — so
//! driving `selected` there compares resting against resting and proves nothing.
//! It is left modelled rather than declared unmodelled because the flag is real
//! on the surface's own default cell, and `Surface::unmodelled` is a per-surface
//! claim rather than a per-cell one.
//!
//! `empty` is real on both shapes: a call site's `size-0` merges the box away,
//! and both engines report `visible: false`. It keeps the legend — the primitive
//! owns that string — so the run stays recorded while the two declarations come
//! off with the measure.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::search_toggle_icons::{ALL_TOGGLES, SearchToggleIcon, Toggle};
use crowbar_ui::components::{AnchorSink, search_toggle_icons};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "search-toggle-icons",
    root: search_toggle_icons::ID_SEARCH_TOGGLE_ICON,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // Both live on the toggle *button*, not on the icon — see the module
        // docs.
        StateFlag::Hover,
        StateFlag::Focus,
    ],
    // An 18px glyph at the base step and `CAPTION_HEIGHT`'s 29. 72 holds both
    // with room, and is a floor rather than a ceiling: this surface drives no
    // height.
    min_window_height: 72,
    // A toggle icon sits inside a 320px search popover — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--toggle`: which of the four entries the cell renders. The one axis
    /// that changes the anchor's *shape*.
    pub toggle: Toggle,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            toggle: SearchToggleIcon::fixture().toggle,
        }
    }
}

impl Params {
    /// The icon this cell describes, built from the live fixture so a bare
    /// `--surface search-toggle-icons` renders the icon the reference has.
    #[must_use]
    pub fn icon(&self, cell: &Cell) -> SearchToggleIcon {
        SearchToggleIcon {
            toggle: self.toggle,
            breakpoint: cell.breakpoint(),
            active: cell.has(StateFlag::Selected),
            empty: cell.has(StateFlag::Empty),
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
            "--toggle" => self.toggle = parse_toggle(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A glyph's height is the button's icon rule and the span's is
    /// its own line box; no option here authors a window height.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · toggle {}", self.toggle.name());
        match self.toggle.legend() {
            Some(legend) => {
                let _ = write!(
                    out,
                    " · a run reading \"{legend}\" — the only shape here the contract can \
                     compare text, colour or font on",
                );
            }
            None => out.push_str(
                " · a phosphor <svg>, drawn as an empty box; its box is the button's \
                 [&_svg:not([class*='size-'])] rule, and nothing else about it is in the \
                 contract",
            ),
        }
        if !self.toggle.in_every_importer() {
            out.push_str(
                " · rendered by find-bar.tsx only, and only while the replace row is open; \
                 terminal-search.tsx offers three options and not this one",
            );
        }
        if cell.has(StateFlag::Selected) {
            if self.toggle.legend().is_some() {
                out.push_str(" · active: the run takes text-foreground");
            } else {
                out.push_str(
                    " · active: the glyph's currentColor moves and NOTHING the contract \
                     records changes with it",
                );
            }
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the box merged away to zero, which both engines report as \
                 visible: false; the legend stays and both declarations come off",
            );
        }
    }

    /// The icon, inside the flex row that makes it a flex item.
    ///
    /// Every live one is a `Button`'s only child, and a button is `inline-flex`
    /// — which is also why the `Aa` span's computed `display` is `block` rather
    /// than `inline`, CSS having blockified it. The row carries no anchor, so it
    /// cannot reach a snapshot.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .justify_center()
            .child(self.icon(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// Which of the four entries. A closed vocabulary, and **no numeric form**: the
/// line P3.1 drew for `--class-radius` holds here too.
fn parse_toggle(raw: &str) -> Result<Toggle, ParseError> {
    ALL_TOGGLES
        .into_iter()
        .find(|toggle| toggle.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!("--toggle takes one of {}, not {raw}", names()))
        })
}

/// The vocabulary as one line, for a usage line and for a rejection.
fn names() -> String {
    ALL_TOGGLES
        .into_iter()
        .map(Toggle::name)
        .collect::<Vec<_>>()
        .join(", ")
}

fn options() -> Vec<(String, String)> {
    [(
        "--toggle <name>".to_owned(),
        format!(
            "one of {} — three are phosphor glyphs the contract can only see the box of, \
             and preserve-case is a text run [{}]",
            names(),
            Params::default().toggle.name(),
        ),
    )]
    .into_iter()
    .collect()
}
