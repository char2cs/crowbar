//! `--surface flicker-spinner` — a flip-dot spinner, and the P3.7 surface where
//! `ANCHORS.md` v1.9 is ruled **out** by a measurement on the anchor itself.
//!
//! The default cell is the agent chat pane's `reviving` spinner, measured live
//! at a 1714px viewport: `24 × 24`, `bg #00000000`, `radius 0`, `border.w 0`,
//! `visible: true`, and **no text group at all**. See
//! `native/mapping/flicker-spinner.md` and `/tmp/p3-ref-flicker-spinner.json`.
//!
//! # v1.9 does not reach this surface, and that is checked rather than assumed
//!
//! The component animates — 25 flip-dots per SVG, 775 `<animate>` elements
//! across the 31 assets — so v1.9 is the first thing to rule out. Two facts do
//! it: every one of those 775 animates **`fill-opacity`**, which no field in the
//! contract records; and `Element.getAnimations()` on the anchored span returned
//! **`[]`** with `transform: none`, so the animation is not on the measured
//! element at all. A capture here is timing-independent in every recorded field.
//!
//! That is the same conclusion P3.6 reached on `skeleton` and the opposite of
//! the one `spinner` gives, which is why the check belongs per component.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--call-site` | **real** — 16 / 16 / 24 / 14 px |
//! | `--theme` | **vacuous.** The dots are `currentColor` and the span paints no text node, so no `fg` is emitted on either side |
//! | `--width` | **vacuous.** Both axes are authored |
//! | `--content` | **vacuous.** A flip-dot spinner paints no text |
//! | `--viewport-width` | **vacuous.** Neither the primitive nor any call site carries a `sm:` variant — checked, because P3.3's trap is what it costs not to |
//!
//! # The state axis
//!
//! Five of the six are unmodelled. `flicker-spinner.tsx` is a class list, a
//! `role` and an `innerHTML`; it has no interaction rule, and a spinner *is* the
//! loading state.
//!
//! `empty` is the exception: a call site merging a `size-0` gives a **zero-area**
//! box that reports `visible: false`, which is `skeleton`'s cell. **No live call
//! site renders it.** The other reading — the `SPINNERS[…] ?? ''` branch that
//! renders the span with no SVG in it — is deliberately not the one modelled,
//! because the SVG is unanchored and that cell would compare identical to the
//! resting one on every recorded field.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::flicker_spinner::{self, ALL_CALL_SITES, CallSite, FlickerSpinner};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "flicker-spinner",
    root: flicker_spinner::ID_FLICKER_SPINNER,
    unmodelled: &[
        // A flip-dot spinner *is* the loading state — `skeleton`'s reason.
        StateFlag::Loading,
        StateFlag::Error,
        // `flicker-spinner.tsx` has no interaction rule of any kind.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The 24px captured box and `CAPTION_HEIGHT`'s 29. A floor rather than a
    // ceiling; this surface drives no height.
    min_window_height: 72,
    // A glyph in a sidebar row or a centred pane state — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the `className` bundle a call site merges over the
    /// primitive's own `size-4`. It carries the box and nothing else.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: FlickerSpinner::fixture().call_site,
        }
    }
}

impl Params {
    /// The spinner this cell describes.
    #[must_use]
    pub fn flicker_spinner(&self, cell: &Cell) -> FlickerSpinner {
        FlickerSpinner {
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

    /// **None.** Every cell's height is its call site's `size-*`, and this
    /// command line sets no other.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · class {}", self.call_site.name());
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: a size-0 call site, so the box has zero area and is not \
                 painted — no live call site renders it",
            );
        }
        match self.call_site {
            CallSite::None => out.push_str(
                " · class: the primitive's own size-4, which agent-chat-glyph.tsx \
                 restates and the other two override",
            ),
            CallSite::ChatPane => out.push_str(" (agent-chat-pane.tsx's — the captured cell)"),
            CallSite::ChatGlyph | CallSite::WorkspaceIcon => {}
        }
        out.push_str(
            " · the dots are unmodelled, and their fill-opacity is outside every field \
             the contract records — so this capture is timing-independent",
        );
    }

    /// The spinner, inside the centred column its live call sites put it in.
    ///
    /// The pane's is `flex h-full w-full flex-col items-center justify-center
    /// gap-3 p-6`; the two row call sites are a `flex … items-center` span. Both
    /// make the glyph a flex item with an authored box, so one column serves.
    /// It carries no anchor.
    fn render(&self, cell: &Cell, _theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .child(self.flicker_spinner(cell).render(anchors))
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
             carries the box and nothing else [{}]",
            names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            FlickerSpinner::fixture().call_site.name(),
        ),
    )]
    .into_iter()
    .collect()
}
