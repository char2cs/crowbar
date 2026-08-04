//! `--surface crowbar-wordmark` — the brand lockup.
//!
//! The default cell is the new-tab pane's isologo, measured live at a 1714px
//! viewport in a `1417 × 1073` pane: `148 × 37.56`, `bg #00000000`, `radius 0`,
//! `border.w 0`, `visible: true`. See `native/mapping/crowbar-wordmark.md` and
//! `/tmp/p3-ref-crowbar-wordmark.json`.
//!
//! # The anchor pins a box, and the lettering is **not** a text run
//!
//! The word "Crowbar" is thirty-one `<path>`s. `native/oracle/ANCHORS.md`'s text
//! group comes off `oracleOwnText(el)`, which walks the element's own text
//! nodes; there are none, so `text`, `text_width`, `clipped`, `font` and `fg`
//! are all **absent**, not empty. The five box fields are the whole comparison.
//! A converged run here proves the lockup is the right size and says nothing
//! about the letterforms.
//!
//! # `--pane-min` is the axis that matters, and it is an input rather than an answer
//!
//! The live width is `clamp(96px, 14cqmin, 148px)` measured against a
//! `container-type: size` pane. `--pane-min` supplies that container's shorter
//! side — the same number CSS reads — and the port resolves the clamp itself.
//! That is P3.1's line: a knob may supply the same input both engines resolve,
//! never the reference's output. On the captured cell the pane is 1073, so
//! `14cqmin` is 150.22 and the **ceiling binds** at 148.
//!
//! The two OOBE lockups read `vw` instead, so on those `--pane-min` is the
//! viewport width. One option rather than two, because each call site has
//! exactly one container-relative term.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--pane-min` | **real** — it moves the clamp, and through the ratio the height with it |
//! | `--theme` | **vacuous.** Every recorded field is theme-invariant; the ink reaches the contract through no field |
//! | `--content` | **vacuous.** The lockup has no content |
//! | `--width` | **vacuous.** The box is the clamp's, not the container's |
//! | `--viewport-width` | **vacuous** on the live call site, whose width is container-relative rather than viewport-relative. It is what `vw` would read on the two OOBE cells, and those are unreachable — so `--pane-min` carries it instead, and this axis moves nothing |
//!
//! # Why the two OOBE lockups have no reference
//!
//! `oobe-screen.tsx` is the `/oobe` route's component, and the only navigation
//! to `/oobe` is `routes/_shell/index.tsx`'s `redirect` when the app has **no
//! projects**. The fixture workspace has projects, so the redirect never fires
//! and a live count of the two lockups is `0`.
//!
//! There is a second wall behind that one, and it is the more interesting:
//! **both lockups sit inside a `motion` layer animating `opacity` from 0**.
//! `ANCHORS.md` v1.7 makes a non-opaque ancestor a capture the driver cannot
//! reproduce, and `oracleAssertComparableOpacity` refuses such a document
//! outright. So even a driven `/oobe` would need the animation settled, and the
//! v1.9 hole — a snapshot cannot say *when* it was taken — applies to it in
//! full. Both are ported and neither is fabricated; `git-row-dir` is the
//! precedent.
//!
//! # The state axis
//!
//! Five of the six are unmodelled: `crowbar-wordmark.tsx` has no interaction
//! rule of any kind, and neither has any of its call sites — the live one is
//! `pointer-events-none` and `aria-hidden`, which is the file saying so out
//! loud. `empty` is the exception and has `crowbar-mark`'s standing exactly: a
//! call site's `size-0` merges the arbitrary width away, and both engines give
//! a zero box zero area and report `visible: false`. No live call site does it.
//! The *bare* primitive — no className at all — is a different picture and is
//! deliberately not modelled; see
//! `crowbar_ui::surfaces::crowbar_wordmark::CallSite`.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::surfaces::crowbar_wordmark::{ALL_CALL_SITES, CallSite, CrowbarWordmark};
use crowbar_ui::AnchorSink;
use crowbar_ui::surfaces::crowbar_wordmark;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, pixels, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "crowbar-wordmark",
    root: crowbar_wordmark::ID_CROWBAR_WORDMARK,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        // No interaction rule anywhere on this component — see the module docs.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The captured lockup is 37.56 tall and the widest modelled one — the OOBE
    // presentation's 360px — is 91.4. 160 holds that and `CAPTION_HEIGHT`'s 29
    // with room, and is a floor rather than a ceiling.
    min_window_height: 160,
    // A lockup sits centred inside a pane — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the `className` bundle that supplies the width.
    pub call_site: CallSite,
    /// `--pane-min`: the length the call site's container-relative term
    /// resolves against — the pane's shorter side for the live cell, the
    /// viewport width for the two OOBE cells.
    pub pane_min: u16,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: CrowbarWordmark::fixture().call_site,
            pane_min: crowbar_wordmark::CAPTURED_PANE_MIN_SIDE,
        }
    }
}

impl Params {
    /// The wordmark this cell describes, built from the live fixture so a bare
    /// `--surface crowbar-wordmark` renders the lockup the reference has.
    #[must_use]
    pub fn wordmark(&self, cell: &Cell) -> CrowbarWordmark {
        CrowbarWordmark {
            call_site: self.call_site,
            basis: px(f32::from(self.pane_min)),
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
            "--pane-min" => self.pane_min = pixels(&value(args, option)?, option)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The lockup's height is its width's ratio, and no option here
    /// authors a window height — the window keeps
    /// [`Surface::min_window_height`](crate::surface::Surface::min_window_height).
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the width merged away to zero, which both engines report as \
                 visible: false; no live call site does it",
            );
            return;
        }
        let width = self.wordmark(cell).width();
        let _ = write!(
            out,
            " · class {} · basis {}px → {:.2} × {:.2}",
            self.call_site.name(),
            self.pane_min,
            f32::from(width),
            f32::from(crowbar_wordmark::height_for(width)),
        );
        match self.call_site {
            CallSite::NewTabView => out.push_str(" (the captured cell)"),
            CallSite::OobePresentation | CallSite::OobeCard => out.push_str(
                " · unreachable: the /oobe route is only entered by the no-projects \
                 redirect, and both lockups sit under a motion opacity layer ANCHORS.md \
                 v1.7 forbids driving through",
            ),
        }
    }

    /// The lockup, inside the centring flex row its live parent is
    /// (`relative flex shrink-0 justify-center pb-5`). The row carries no
    /// anchor.
    fn render(&self, cell: &Cell, _theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .justify_center()
            .child(self.wordmark(cell).render(anchors))
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
                "--call-site takes one of {}, not {raw}; it names the className bundle a \
                 call site merges, never a pixel value",
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
    [
        (
            "--call-site <name>".to_owned(),
            format!(
                "one of {} — the className bundle that supplies the width, never a pixel \
                 value; the primitive authors none [{}]",
                names(),
                Params::default().call_site.name(),
            ),
        ),
        (
            "--pane-min <px>".to_owned(),
            format!(
                "the length the call site's container-relative term resolves against: the \
                 pane's shorter side for 14cqmin, the viewport width for vw [{}]",
                Params::default().pane_min,
            ),
        ),
    ]
    .into_iter()
    .collect()
}
