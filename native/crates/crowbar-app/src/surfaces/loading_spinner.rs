//! `--surface loading-spinner` — a glyph, a gap and a caption, and the only
//! compound of the three P3.7 spinners.
//!
//! The default cell is a freshly-mounted **commit** `ReviewDiffTab`, measured
//! live at a 1714px viewport: wrapper `138 × 18`, glyph `16 × 16` at `y 1`,
//! caption `116 × 18` at `x 22` reading `Loading commit diff`, `fg #a4a4a4ff`,
//! `CalSansUI` 12/18 at weight 400. See `native/mapping/loading-spinner.md` and
//! `/tmp/p3-ref-loading-spinner.json`.
//!
//! # v1.9 reaches one of the three anchors, not the surface
//!
//! `transform` does not participate in layout, so the wrapper's border box is
//! unmoved by the glyph spinning inside it and the caption animates nothing at
//! all. Only the nested `spinner` anchor is in flight — by 6.63px, which is
//! thirteen times §5's tolerance. The reference was captured with the animation
//! pinned at `currentTime = 0` for that one anchor's sake; the other two would
//! have been right at any instant.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--call-site` | **real** — it carries the caption's string, whether there is one, and `compact` |
//! | `--theme` | **real**: `text-muted-foreground` is a different token in the two tables |
//! | `--content` | **vacuous.** The caption's string is the call site's prop, not a fixture length — see below |
//! | `--width` | **vacuous.** The wrapper is `inline-flex` with no authored width and no stretch |
//! | `--viewport-width` | **vacuous.** Neither `loading-spinner.tsx` nor the caption carries a `sm:` variant |
//!
//! **`--content` is deliberately not wired to the caption.** The four live
//! captions are four different `label` props at four different call sites, so
//! the string and the call site are one quantity; a `--content` that swapped
//! `Loading commit diff` for a longer string of this surface's invention would
//! be painting text the product never shows. `--call-site` moves the run's
//! width instead, and it moves it between strings that exist.
//!
//! # The state axis
//!
//! Five of the six are unmodelled, for `spinner`'s reasons — this component adds
//! no interaction rule of its own.
//!
//! `empty` is the exception and it is **live** here, unlike on the other two: a
//! `<LoadingSpinner>`'s own content is its caption, and the `<Suspense>`
//! fallback and the LSP status chip both render it without one. The cell drops
//! an anchor rather than a field, which is the loudest difference the differ
//! has, and it is the same picture `--call-site fallback` reaches from the other
//! direction.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::AnchorSink;
use crowbar_ui::components::loading_spinner::{self, ALL_CALL_SITES, CallSite, LoadingSpinner};
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Styled as _, div};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "loading-spinner",
    root: loading_spinner::ID_ROOT,
    unmodelled: &[
        // A loading spinner *is* the loading state — `skeleton`'s reason.
        StateFlag::Loading,
        StateFlag::Error,
        // Neither this component nor the glyph inside it has an interaction
        // rule of any kind.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // An 18px line box and `CAPTION_HEIGHT`'s 29. A floor rather than a ceiling;
    // this surface drives no height.
    min_window_height: 72,
    // A centred state inside a pane — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--call-site`: the props bundle a call site passes. It carries the
    /// caption's string, whether the caption is painted, and `compact` — which
    /// moves the gap **and** the glyph.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            call_site: LoadingSpinner::fixture().call_site,
        }
    }
}

impl Params {
    /// The spinner this cell describes.
    #[must_use]
    pub fn loading_spinner(&self, cell: &Cell) -> LoadingSpinner {
        LoadingSpinner {
            call_site: self.call_site,
            empty: cell.has(StateFlag::Empty),
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

    /// **None.** The wrapper's height is its tallest item's, and this command
    /// line sets no other.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let cell_spinner = self.loading_spinner(cell);
        let _ = write!(out, " · props {}", self.call_site.name());
        match cell_spinner.label() {
            Some(text) => {
                let _ = write!(out, " · caption \"{text}\"");
            }
            None => out.push_str(
                " · icon only: the label prop reaches the glyph's aria-label, which no \
                 field in the contract records",
            ),
        }
        if self.call_site.compact() {
            out.push_str(" · compact: a 4px gap and a 12px glyph, both");
        }
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: the caption is dropped, so the snapshot loses an anchor rather \
                 than a field — the same picture --call-site fallback reaches",
            );
        }
        out.push_str(
            " · captured at rest: the nested spinner anchor is animated and the other two \
             are not",
        );
    }

    /// The spinner, inside the centred column every live call site puts it in.
    ///
    /// `CenteredState` in `review-diff-tab.tsx` is `flex h-full flex-col
    /// items-center justify-center`, and the `items-center` is what makes the
    /// wrapper shrink to its content — which is why the reference's wrapper is
    /// 138 wide rather than the pane's. The column carries no anchor.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .child(self.loading_spinner(cell).render(theme, anchors))
            .into_any_element()
    }
}

/// A call site's props bundle.
///
/// **There is deliberately no free-text `--label`**, the line P3.1 drew for
/// `--class-radius` applied to a string: a knob may supply the same *input* both
/// engines resolve, and the four captions this component paints are the four
/// props four call sites pass. A caption of the caller's invention would be a
/// run the product never shapes.
fn parse_call_site(raw: &str) -> Result<CallSite, ParseError> {
    ALL_CALL_SITES
        .into_iter()
        .find(|site| site.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--call-site takes one of {}, not {raw}; it names the props bundle a call \
                 site passes, never a caption of your own",
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
            "one of {} — the props bundle a call site passes; it carries the caption's \
             string, whether one is painted, and compact [{}]",
            names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
            LoadingSpinner::fixture().call_site.name(),
        ),
    )]
    .into_iter()
    .collect()
}
