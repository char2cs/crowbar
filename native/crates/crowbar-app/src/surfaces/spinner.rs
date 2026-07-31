//! `--surface spinner` — a rotating glyph, and **the first surface in the port
//! where `ANCHORS.md` v1.9 decides the run**.
//!
//! The default cell is the `<Spinner>` inside a freshly-mounted commit
//! `ReviewDiffTab`, measured live at a 1714px viewport: `16 × 16`, `bg
//! #00000000`, `radius 0`, `border.w 0`, and **no text group at all**. See
//! `native/mapping/spinner.md` and `/tmp/p3-ref-spinner.json`.
//!
//! # The capture instant is load-bearing here, and that is a measurement
//!
//! `animate-spin` puts `transform: rotate(…)` on the anchored element, and
//! `transform` moves `getBoundingClientRect()` — which is the exact call the
//! React extractor's `bounds` come from. Stepping the live animation's own
//! timeline gave 16.000 at 0°, **22.627 at 45°** and 16.000 again at 90°: a
//! 6.63px excursion on `bounds.w`, `bounds.h`, `bounds.x` and `bounds.y`,
//! against §5's ±0.5px.
//!
//! Four instants per second are at rest and the other 996 milliseconds are not.
//! The reference was therefore taken with the animation pinned at
//! `currentTime = 0`; **anyone re-capturing this surface has to do the same**,
//! and a run that comes back with four deltas on the only anchor should suspect
//! the instant before the port. See `crowbar_ui::components::spinner`.
//!
//! # The native side turns, and needs no pinning at all
//!
//! `Spinner` draws lucide's arc and rotates it once a second, because a spinner
//! that does not spin is the most noticeable thing a user could find. That costs
//! this surface nothing, for two reasons `row_layout::spinner` measures rather
//! than asserts by hand:
//!
//! * this binary emits **the first frame** and quits, and gpui stamps an
//!   animation's `start` on its first `request_layout` — so the capture is at
//!   `delta ≈ 0` by construction;
//! * gpui rotates at **paint** time, so the *layout* bounds the driver records
//!   are the same at every delta anyway.
//!
//! Verified end to end: the emitted snapshot is byte-identical to the one the
//! pre-rotation port produced, and six consecutive runs produced one distinct
//! file.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--call-site` | **real** — 24 / 16 / 12 / 18-or-16 px |
//! | `--viewport-width` | **real for `--call-site button-loading-indicator` only.** That call site carries no `size-*`, so the button's own `[&_svg…]:size-4.5 sm:[&_svg…]:size-4` sizes it |
//! | `--theme` | **vacuous.** The glyph's only colour is `currentColor`, and an anchor with no text node emits no `fg` — nothing here is a compared colour |
//! | `--width` | **vacuous.** Both axes are authored |
//! | `--content` | **vacuous.** A spinner paints no text |
//!
//! # The state axis
//!
//! Five of the six are unmodelled. `spinner.tsx` is one `cn()` and a `role`; it
//! has no interaction rule of any kind, and a spinner *is* the loading state so
//! `loading` has no resting rendering to differ from — `skeleton`'s reason.
//!
//! `empty` is the exception and is a real branch of the primitive:
//! `<Spinner size={0} />` writes `width="0" height="0"` with no `size-*` class
//! to override them, so the box has **zero area** and reports `visible: false`.
//! It is the same picture `skeleton`'s `empty` reaches, from the props rather
//! than from the absence of a class, and **no live call site renders it**.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::spinner::{ALL_CALL_SITES, CallSite, Extent, Spinner};
use crowbar_ui::components::{AnchorSink, spinner};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "spinner",
    root: spinner::ID_SPINNER,
    unmodelled: &[
        // A spinner *is* the loading state — see the module docs.
        StateFlag::Loading,
        StateFlag::Error,
        // `spinner.tsx` has no interaction rule of any kind.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The bare primitive's 24px box and `CAPTION_HEIGHT`'s 29. A floor rather
    // than a ceiling; this surface drives no height.
    min_window_height: 72,
    // A spinner sits inside a status chip or a centred state — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the `className` bundle a call site merges over the
    /// primitive's own. It carries the box, and on one of the four it carries a
    /// breakpoint with it.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: match Spinner::fixture().extent {
                Extent::Class(call_site) => call_site,
                // Unreachable while the fixture is the captured cell, and a
                // `None` default would be a cell no live call site renders.
                Extent::Empty => CallSite::LoadingSpinner,
            },
        }
    }
}

impl Params {
    /// The glyph this cell describes.
    #[must_use]
    pub fn spinner(&self, cell: &Cell) -> Spinner {
        Spinner {
            // `empty` overrides the call site: §8.3's word is "a surface with
            // nothing in it", and a glyph with an authored box is not that.
            extent: if cell.has(StateFlag::Empty) {
                Extent::Empty
            } else {
                Extent::Class(self.call_site)
            },
            breakpoint: cell.breakpoint(),
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

    /// **None.** Every cell's height is its call site's `size-*`, and this
    /// command line sets no other.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · class {}", self.call_site.name());
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: size={0}, so the box has zero area and is not painted — \
                 no live call site renders it",
            );
        }
        match self.call_site {
            CallSite::None => out.push_str(
                " · class: lucide's own 24px intrinsic size, which both importers merge \
                 something over, so there is no reference",
            ),
            CallSite::ButtonLoadingIndicator => out.push_str(
                " · dead: no <Button> in web/src is ever passed loading, and this is the \
                 one call site --viewport-width moves (18 below 640px, 16 at or above)",
            ),
            CallSite::LoadingSpinner => {
                out.push_str(" (loading-spinner.tsx's size-4 — the captured cell)");
            }
            CallSite::LoadingSpinnerCompact => {}
        }
        let _ = write!(
            out,
            " · turning: this frame is the turn's origin, and the REFERENCE must be pinned \
             at currentTime 0 — its bounds move {:.2}px over a turn where these do not",
            f32::from(self.spinner(cell).rotation_excursion()),
        );
    }

    /// The glyph, inside the flex row it is a child of at every live call site.
    ///
    /// The row carries no anchor, so the snapshot holds exactly one record. It
    /// is a real row rather than a bare box for `label`'s reason: `RowSurface`
    /// draws into a gpui **block** container, and every live `<Spinner>` is a
    /// flex item.
    ///
    /// The row carries the **host's** text colour, because the glyph strokes
    /// itself in `currentColor` and the captured one inherits
    /// `text-muted-foreground` from `review-diff-tab.tsx`'s `CenteredState` —
    /// measured live as `oklch(0.72 0 0)`. It moves no recorded field, an anchor
    /// with no text node emitting no `fg`, which is why it is set deliberately:
    /// an arc stroked in an invisible colour would look like a static box and no
    /// gate would notice.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_row()
            .items_center()
            .text_color(theme.color_muted_foreground)
            .child(self.spinner(cell).render(anchors))
            .into_any_element()
    }
}

/// A call site's `className` bundle.
///
/// **There is deliberately no numeric form**, the line P3.1 drew for
/// `--class-radius`: a knob may supply the same *input* both engines resolve,
/// never the reference's *output*.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the className bundle a \
                 call site merges, never a pixel value",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            ))
        })
}

/// A vocabulary as one line, for a usage line and for a rejection.
fn names<I: Iterator<Item = &'static str>>(words: I) -> String {
    words.collect::<Vec<_>>().join(", ")
}

fn options() -> Vec<(String, String)> {
    [(
        "--call-site <name>".to_owned(),
        format!(
            "one of {} — the className bundle a call site merges, never a pixel value; it \
             carries the box, and on button-loading-indicator the breakpoint too [{}]",
            names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            Params::default().call_site.name(),
        ),
    )]
    .into_iter()
    .collect()
}
