//! `--surface inline-error` — a retry panel with **no reference**, because its
//! render guard cannot become true.
//!
//! See `crowbar_ui::components::inline_error` for the measurement that
//! establishes that, and `native/mapping/inline-error.md`. The short form: the
//! only writer of the store's error state fires when the fetcher rejects, and the
//! fetcher's entire I/O is `try { … } catch { return [] }`. Confirmed in the
//! running app by breaking `IndexedDB` two different ways and watching both reads
//! come back through the catch arm.
//!
//! The values are the utilities', resolved through the app's own compiled
//! `tailwindcss` 4.3.0 and read back off a probe element in the live document.
//! **Nothing here was captured as a snapshot and no reference JSON exists** —
//! `git-row-dir`, `separator` and `skeleton` are the precedent.
//!
//! # The default `--width` is 294, and that is not arbitrary
//!
//! The panel's only call site is `workspace-tree.tsx`, inside the sidebar. The
//! live sidebar measures **294px**, so that is what the probe was taken at and
//! what this surface defaults to. It matters more here than on a leaf: the panel
//! is `p-6` with `items-center`, so the content box is 246px and the detail line
//! is the string most likely to wrap in it. P3.4's `input` lost a whole run to
//! exactly this — a `--width` left at its default while the reference's container
//! was narrower, and every anchor off by the same constant.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--width` | **real** — the panel stretches, and its three centred children re-centre inside it |
//! | `--content` | **real** — it moves the detail run, the one string a caller does not control |
//! | `--theme` | **real**: `text-foreground` and `text-muted-foreground` both differ in the two tables |
//! | `--viewport-width` | **real** — `h-8 sm:h-7` moves the retry control's height at 640px, and it is the only thing on this surface that does |
//!
//! # The state axis
//!
//! `empty` is the **production build**: the detail line sits behind
//! `import.meta.env.DEV`, so a shipped panel has four anchors where a dev one has
//! five. That is a genuine branch in the anchor *set* and is why this surface
//! declares no set on the reference side (`ANCHORS.md` v1.8).
//!
//! `error` is unmodelled, and the word is doing something confusing here so it is
//! worth being explicit: this surface **is** the app's error state, and there is
//! no second error inside it to drive. `surface.rs` requires every surface to
//! declare `Error` unmodelled, and `input` recorded the same shape.
//!
//! `hover`, `focus` and `selected` are unmodelled on the panel — but note the
//! retry `<Button>` has all three in `button.tsx`. They are not modelled here
//! because the control is composed from `button`'s *resting* values; a cell that
//! drove them would move a box this surface does not own, and `button`'s own
//! surface is where that belongs.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::components::inline_error::{self, InlineError};
use crowbar_ui::components::{AnchorSink, ContentLength};
use gpui::{AnyElement, SharedString};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "inline-error",
    root: inline_error::ID_PANEL,
    unmodelled: &[
        StateFlag::Loading,
        // The surface *is* the error state — see the module docs.
        StateFlag::Error,
        // `inline-error.tsx` carries no interaction rule of its own; the three
        // the retry control has belong to `button`'s surface.
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // The probe measures the panel at 128 tall in the sidebar, plus
    // `CAPTION_HEIGHT`'s 29. A floor rather than a ceiling: the panel is
    // `flex-1` and takes whatever column it is given.
    min_window_height: 180,
    // The panel fills the sidebar, not the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--title`: the `title` prop, whose default is the only value any live
    /// call site produces — `workspace-tree.tsx` passes none.
    pub title: SharedString,
}

impl Default for Params {
    fn default() -> Self {
        Self {
            title: InlineError::fixture().title,
        }
    }
}

impl Params {
    /// The panel this cell describes.
    #[must_use]
    pub fn panel(&self, cell: &Cell) -> InlineError {
        InlineError {
            title: self.title.clone(),
            detail: if cell.has(StateFlag::Empty) {
                None
            } else {
                Some(detail_of(cell.content))
            },
            breakpoint: cell.breakpoint(),
        }
    }
}

/// The `error.message` a content length shows.
///
/// **Every one is a single unbreakable token**, for the usual reason: the
/// sidebar leaves a 246px content box and a wrapped run is uncomparable — the DOM
/// sums client rects where gpui shapes one line.
///
/// # A hyphen is a break opportunity, and it cost a test
///
/// The `overflow` string was first written
/// `ECONNREFUSED-while-reading-the-workspace-entity-cache`, which has no spaces
/// and **still wrapped**: `U+002D` is line-break class `HY`, so both engines
/// break after it. Measured — the detail box came back 33px tall against 16.5
/// for the other two, exactly two lines.
///
/// "Unbreakable" therefore means no spaces **and no hyphens**, which is a
/// stronger condition than the one the brief states and than the one a
/// single-word fixture happens to satisfy by accident.
///
/// # The `<p>` cannot clip, so `overflow` here is a wrap test and not a clip test
///
/// `inline-error.tsx`'s detail line carries no `truncate` and no
/// `overflow-hidden`, so `clipped` is `false` at every length and the box grows
/// instead. That makes `overflow` on this surface a check that the run still
/// shapes on one line, which is what the assertion in `row_layout` measures.
///
/// # Why `overflow` still **fits**, which looks wrong and is not
///
/// A run genuinely wider than the 246px content box is **uncomparable on this
/// surface**, and not because of the fixture: gpui wraps such a run where `WebKit`,
/// with `overflow-wrap: normal` and no break opportunity, lets it *overflow* on
/// one line. The two engines then disagree by a whole line box on a field the
/// contract compares exactly. Measured — a 47-character token came back 33px tall
/// against 16.5 for a shorter one.
///
/// So `overflow` here is the **longest run that still shapes on one line**, which
/// keeps all three content cells comparable. The genuinely-too-long case is
/// recorded as a divergence rather than driven, and
/// `an_unbreakable_run_wider_than_the_box_wraps_here_and_would_not_in_webkit`
/// pins it so the fact stays measured.
fn detail_of(content: ContentLength) -> SharedString {
    match content {
        ContentLength::Short => SharedString::new_static("offline"),
        ContentLength::Normal => SharedString::new_static("ECONNREFUSED"),
        ContentLength::Overflow => SharedString::new_static("ECONNREFUSEDreadingthecache"),
    }
}

impl SurfaceParams for Params {
    fn accept(
        &mut self,
        option: &str,
        args: &mut dyn Iterator<Item = String>,
    ) -> Result<bool, ParseError> {
        match option {
            "--title" => {
                let raw = value(args, option)?;
                if raw.is_empty() {
                    return Err(ParseError::Rejected(
                        "--title takes a string; the prop defaults rather than \
                         rendering an empty heading"
                            .to_owned(),
                    ));
                }
                self.title = SharedString::from(raw);
            }
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** The panel is `flex-1` and takes the column it is given.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let _ = write!(out, " · \"{}\"", self.title);
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: a production build, so the dev-only detail line and its \
                 anchor are both absent",
            );
        } else {
            let _ = write!(out, " · detail \"{}\"", detail_of(cell.content));
        }
        out.push_str(" · no reference: the store's error state is unreachable");
    }

    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        self.panel(cell).render(theme, anchors)
    }
}

fn options() -> Vec<(String, String)> {
    [(
        "--title <text>".to_owned(),
        format!(
            "the `title` prop; no live call site passes one, so the default is the \
             only string the app produces [{}]",
            InlineError::fixture().title,
        ),
    )]
    .into_iter()
    .collect()
}
