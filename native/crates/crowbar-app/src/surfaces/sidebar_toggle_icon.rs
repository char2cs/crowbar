//! `--surface sidebar-toggle-icon` — the panel glyph.
//!
//! The default cell is the sidebar header's toggle, measured live at a 1714px
//! viewport: `16 × 16`, `bg #00000000`, `radius 0`, `border.w 0`,
//! `visible: true`. See `native/mapping/sidebar-toggle-icon.md` and
//! `/tmp/p3-ref-sidebar-toggle-icon.json`.
//!
//! # The anchor pins a box, and the glyph is the part it cannot see
//!
//! A rounded rect, a divider and two rail lines, stroked at 2px in
//! `currentColor`. `native/oracle/ANCHORS.md` has no field for `stroke`,
//! `stroke-width` or path data, and `fg` comes off the element's own text
//! nodes — an `<svg>` has none. **The rect's `rx="2.5"` is not the anchor's
//! `radius`** either: that field reads CSS `border-radius`, which is 0. A port
//! that translated the rect corner would paint a real one and the differ would
//! call it, which is this component's one way of failing in the direction its
//! invisibility usually protects.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--viewport-width` | **vacuous, and that is the finding.** The primitive carries `size-4`, which opts it out of `button`'s `[&_svg:not([class*='size-'])]` rule — the one thing on this glyph a breakpoint would otherwise move, 18 → 16 |
//! | `--theme` | **vacuous.** Every recorded field is theme-invariant; the stroke reaches the contract through no field |
//! | `--content` | **vacuous.** No content |
//! | `--width` | **vacuous.** `size-4` authors both axes |
//!
//! # The state axis
//!
//! Five of the six are unmodelled: `sidebar-toggle-icon.tsx` has **no
//! interaction rule of any kind**, and its two call sites put theirs on the
//! button — `hover:bg-sidebar-element-hover` moves the *button's* background,
//! which is a different anchor on a different surface.
//!
//! `empty` is the exception and has `crowbar-mark`'s standing: a call site's
//! `size-0` **replaces** the primitive's `size-4` — `cn` is tailwind-merge —
//! and both engines give the resulting zero box no area and report
//! `visible: false`. No live call site does it.

use crowbar_ui::Theme;
use crowbar_ui::components::sidebar_toggle_icon::{ALL_CALL_SITES, CallSite, SidebarToggleIcon};
use crowbar_ui::components::{AnchorSink, sidebar_toggle_icon};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "sidebar-toggle-icon",
    root: sidebar_toggle_icon::ID_SIDEBAR_TOGGLE_ICON,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // No interaction rule on the glyph — its call sites style the button.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // A 16px glyph and `CAPTION_HEIGHT`'s 29. 72 holds both with room, and is a
    // floor rather than a ceiling: this surface drives no height.
    min_window_height: 72,
    // A toggle glyph sits inside a button — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: which importer's className is merged over the
    /// primitive's `size-4`. **All three merge nothing**, which is the
    /// measurement rather than an oversight.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: SidebarToggleIcon::fixture().call_site,
        }
    }
}

impl Params {
    /// The glyph this cell describes, built from the live fixture so a bare
    /// `--surface sidebar-toggle-icon` renders the glyph the reference has.
    ///
    #[must_use]
    pub fn icon(&self, cell: &Cell) -> SidebarToggleIcon {
        SidebarToggleIcon {
            call_site: self.call_site,
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
            "--call-site" => self.call_site = parse_call_site(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** `size-4` authors the height and no option here moves it.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: size-4 merged away to zero, which both engines report as \
                 visible: false; no live call site does it",
            );
            return;
        }
        out.push_str(" · class ");
        out.push_str(self.call_site.name());
        match self.call_site {
            CallSite::None => out.push_str(" (the bare primitive — the same 16px box)"),
            CallSite::TabNavigation => {
                out.push_str(" (the tab bar's toggle, shown while the sidebar is hidden)");
            }
            CallSite::SidebarProjectHeader => out.push_str(" (the captured cell)"),
        }
        out.push_str(
            " · size-4 opts out of the button's icon rule, so the extent is 16 at every \
             viewport width",
        );
    }

    /// The glyph, inside the flex row that makes it a flex item — every live
    /// one is a `Button`'s child, and a button is `inline-flex`. The row carries
    /// no anchor.
    fn render(&self, cell: &Cell, _theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .justify_center()
            .child(self.icon(cell).render(anchors))
            .into_any_element()
    }
}

/// A call site's `className` bundle. No numeric form — see
/// `crate::surfaces::crowbar_mark`'s note, which draws the same line.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the importer whose \
                 className is merged, never a pixel value",
                names(),
            ))
        })
}

/// The vocabulary as one line, for a usage line and for a rejection.
fn names() -> String {
    ALL_CALL_SITES
        .into_iter()
        .map(CallSite::name)
        .collect::<Vec<_>>()
        .join(", ")
}

fn options() -> Vec<(String, String)> {
    [(
        "--call-site <name>".to_owned(),
        format!(
            "one of {} — the importer whose className is merged over the primitive's \
             size-4; all three merge nothing, which is the measurement [{}]",
            names(),
            Params::default().call_site.name(),
        ),
    )]
    .into_iter()
    .collect()
}
