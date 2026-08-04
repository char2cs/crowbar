//! `--surface crowbar-mark` — the brand mark, and the first P3.8 icon.
//!
//! The default cell is the tab bar's `newTab` icon, measured live at a 1714px
//! viewport: `18 × 18`, `bg #00000000`, `radius 0`, `border.w 0`,
//! `visible: true`. See `native/mapping/crowbar-mark.md` and
//! `/tmp/p3-ref-crowbar-mark.json`.
//!
//! # The anchor pins a box, and that is the **whole** of its coverage
//!
//! `crowbar-mark.tsx` is an `<svg>` and nothing else. `native/oracle/ANCHORS.md`
//! §3 has no field for path data, `fill`, or `preserveAspectRatio`, and `fg`
//! comes off the element's own text nodes, of which an `<svg>` has none. So the
//! five fields the reference carries — `bounds`, `bg`, `visible`, `radius`,
//! `border` — are the complete comparison, and the mark's actual art is
//! unmeasured on both sides. Said plainly here because a converged run on this
//! surface proves the box and says nothing at all about the picture.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--theme` | **vacuous.** Every recorded field is theme-invariant: the box is transparent, and the mark's ink reaches the contract through no field |
//! | `--content` | **vacuous.** The mark has no content; its paths are fixed |
//! | `--width` | **vacuous.** The box is a pinned extent, not a stretch |
//! | `--viewport-width` | **vacuous.** `crowbar-mark.tsx` has no `sm:` variant, and neither does its call site's `size-[18px]` |
//!
//! That table is the finding rather than an apology: **this surface's whole
//! §8.3 matrix is one picture**, and the only input that moves a recorded field
//! is whether the mark has a box at all.
//!
//! # The state axis
//!
//! Five of the six are unmodelled: neither the primitive nor the tab bar's call
//! site carries an interaction rule for the mark — the sibling
//! `FileExplorerIcon` takes an `isActive` colour and **the mark deliberately
//! does not**, it is pinned `text-muted-foreground` in every tab state.
//!
//! `empty` is the exception, and it is a real branch rather than a contrivance:
//! the mark authors no box, so a call site's `size-0` would merge its extent
//! away entirely — `cn` is tailwind-merge — and both engines give a zero box
//! zero area and report `visible: false`. No live call site does it; the
//! rendering is comparable if one ever does, which is more than can be said for
//! the *bare* primitive, whose SVG `width: auto` default this port deliberately
//! does not model (see `crowbar_ui::surfaces::crowbar_mark`).

use crowbar_ui::Theme;
use crowbar_ui::surfaces::crowbar_mark::CrowbarMark;
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::crowbar_mark;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "crowbar-mark",
    root: crowbar_mark::ID_CROWBAR_MARK,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // Neither the primitive nor its call site carries an interaction rule
        // for the mark — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // An 18px mark and `CAPTION_HEIGHT`'s 29. 72 holds both with room, and is a
    // floor rather than a ceiling: this surface drives no height.
    min_window_height: 72,
    // A tab icon sits inside a tab — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
///
/// **No `--call-site`**, and that is the measurement: `tab-bar-item.tsx` is the
/// only importer of `<CrowbarMark>` in the tree, so a vocabulary here would be
/// one word with nothing to choose between. The 18px extent lives in
/// `crowbar_ui::surfaces::crowbar_mark`, where it is reviewable, rather than
/// on a command line where it would hand the port the reference's answer.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--in-slot`: draw the tab bar's `grid size-3.5 place-content-center`
    /// slot around the mark, which the mark **overflows**. The slot carries no
    /// anchor.
    pub in_slot: bool,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            in_slot: CrowbarMark::fixture().in_slot,
        }
    }
}

impl Params {
    /// The mark this cell describes, built from the live fixture so a bare
    /// `--surface crowbar-mark` renders the mark the reference actually has.
    #[must_use]
    pub fn mark(&self, cell: &Cell) -> CrowbarMark {
        CrowbarMark {
            in_slot: self.in_slot,
            empty: cell.has(StateFlag::Empty),
        }
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        _args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--in-slot" => self.in_slot = true,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The mark's height is its call site's extent, and no option here
    /// authors a window height.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the extent merged away to zero, which both engines report as \
                 visible: false; no live call site does it",
            );
        } else {
            out.push_str(" · size-[18px], the tab bar's only call site (the captured cell)");
        }
        if self.in_slot {
            out.push_str(
                " · in the tab bar's 14px slot, which the 18px mark overflows on purpose; \
                 the slot carries no anchor",
            );
        }
    }

    /// The mark, inside the flex row that makes it a flex item.
    ///
    /// `RowSurface` draws every surface into a gpui **block** container, and the
    /// mark's live parent is a centring grid; a flex row reproduces the same
    /// arrangement for a single child. The row carries no anchor, so it cannot
    /// reach a snapshot.
    fn render(&self, cell: &Cell, _theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .child(self.mark(cell).render(anchors))
            .into_any_element()
    }
}

fn options() -> Vec<(String, String)> {
    [(
        "--in-slot".to_owned(),
        "draw the tab bar's 14px grid slot around the mark, which the 18px mark \
         overflows; the slot carries no anchor [off]"
            .to_owned(),
    )]
    .into_iter()
    .collect()
}
