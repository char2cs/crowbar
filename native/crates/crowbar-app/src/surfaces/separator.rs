//! `--surface separator` — a one-pixel rule, and a surface with **no
//! reference**.
//!
//! The default cell is `ToolbarGroup`'s vertical rule. It is not a captured
//! fixture: every `<Separator>` in the tree is Plate editor chrome behind a
//! *focused-editor* gate, and `document.hasFocus()` is false and immovable in
//! this webview — the same measurement that blocks every `:focus` cell in
//! `native/oracle/blocked/hover-and-focus-need-an-unlocked-screen.md`. A live
//! count of `[data-slot="separator"]` was **0** in every reachable state. The
//! reasoning is at `crowbar_ui::primitives::separator`; see also
//! `native/mapping/separator.md`.
//!
//! **Every value the surface renders is the app's own compiled CSS**, so this is
//! `git-row-dir`'s situation with the sign flipped — rendered by the port and
//! absent from the reachable product — rather than a guess.
//!
//! # A vertical rule needs a host with a height, and the host is not an anchor
//!
//! `self-stretch` resolves against the flex line, so a vertical separator drawn
//! into an unconstrained container has no height at all. This surface therefore
//! draws its rule inside a fixed-height row — `ToolbarGroup`'s own `py-0.5`
//! wrapper is the live shape — and that row carries **no anchor**, so it cannot
//! reach a snapshot. [`HOST_HEIGHT`] is the number, and it is the surface's
//! choice rather than the component's: nothing in `separator.tsx` names a
//! height, which is exactly what `self-stretch` means.
//!
//! # What each axis can and cannot do here
//!
//! | Axis | Here |
//! |---|---|
//! | `--theme` | **real**: `bg-border` is a different token in the two tables |
//! | `--width` | **real for a horizontal rule only** — `w-full` takes the host's width. A vertical rule is one pixel at every width |
//! | `--content` | **vacuous.** A separator paints no text |
//! | `--viewport-width` | **vacuous.** `separator.tsx` contains no `sm:` variant |
//!
//! # The state axis
//!
//! Five of the six are unmodelled: `separator.tsx` has no interaction rule and
//! no optional content. It is two class lists selected by `data-orientation`.
//!
//! `empty` is the exception, and on this surface it means the thing
//! `self-stretch` is *for*: a host row with no other content. The flex line's
//! cross size is then the rule's own — which is `auto`, so a stretched vertical
//! rule collapses to **zero height** and reports `visible: false`. That is a
//! real branch of the one property this component has that a port can get
//! wrong, gpui's `align_self: Stretch` and CSS's being two implementations of
//! the same word. `ToolbarGroup` reaches the same picture from the other
//! direction — it is `hidden has-[button]:flex`, so an empty group is
//! `display: none` and its rule is not painted either.

use std::fmt::Write as _;

use crowbar_ui::Theme;
use crowbar_ui::primitives::separator::{
    ALL_CALL_SITES, ALL_ORIENTATIONS, CallSite, Orientation, Separator,
};
use crowbar_ui::AnchorSink;
use crowbar_ui::primitives::separator;
use gpui::{AnyElement, IntoElement as _, ParentElement as _, Pixels, Styled as _, div, px};

use crate::row_surface::{Cell, ParseError, StateFlag};
use crate::surface::{Surface, SurfaceParams, value};

/// The registry entry `build.rs` collects.
pub static SURFACE: Surface = Surface {
    name: "separator",
    root: separator::ID_SEPARATOR,
    unmodelled: &[
        StateFlag::Loading,
        StateFlag::Error,
        StateFlag::Hover,
        StateFlag::Focus,
        StateFlag::Selected,
    ],
    // [`HOST_HEIGHT`] plus `CAPTION_HEIGHT`'s 29, with room. A floor rather
    // than a ceiling; this surface drives no height.
    min_window_height: 72,
    // A rule sits inside a toolbar — never the window.
    full_bleed: false,
    options,
    params: || Box::new(Params::default()),
};

/// The height of the row a vertical rule stretches inside.
///
/// The surface's number, not the component's — see the module docs. 24px is
/// `ToolbarGroup`'s button row, which is what the live rule stretches against.
pub const HOST_HEIGHT: Pixels = px(24.0);

/// This surface's own options.
#[derive(Clone, Debug, PartialEq)]
pub struct Params {
    /// `--orientation`: which way the rule runs.
    pub orientation: Orientation,
    /// `--call-site`: the `className` bundle a call site merges. No live one
    /// pins a height, which is the branch this option exists to make visible.
    pub call_site: CallSite,
}

impl Default for Params {
    fn default() -> Self {
        let fixture = Separator::fixture();
        Self {
            orientation: fixture.orientation,
            call_site: fixture.call_site,
        }
    }
}

impl Params {
    /// The separator this cell describes.
    #[must_use]
    pub fn separator(&self) -> Separator {
        Separator {
            orientation: self.orientation,
            call_site: self.call_site,
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
            "--call-site" => self.call_site = parse_call_site(&value(args, option)?)?,
            _ => return Ok(false),
        }
        Ok(true)
    }

    /// **None.** A horizontal rule is one pixel and a vertical one takes its
    /// host's height; neither is a number this command line sets.
    fn driven_height(&self, _cell: &Cell) -> Option<u16> {
        None
    }

    fn describe(&self, cell: &Cell, out: &mut String) {
        let separator = self.separator();
        let _ = write!(
            out,
            " · {} · class {}",
            separator.orientation.name(),
            self.call_site.name(),
        );
        if cell.has(StateFlag::Empty) {
            out.push_str(
                " · empty: a host with no content, so a stretched rule collapses to zero \
                 height and is not painted",
            );
        } else if separator.stretches() {
            let _ = write!(
                out,
                " · self-stretch against a {}px host",
                f32::from(HOST_HEIGHT),
            );
        }
        out.push_str(
            " · no reference: every <Separator> is Plate chrome behind a focused-editor \
             gate, and document.hasFocus() is false in this webview",
        );
    }

    /// The rule, inside the row it stretches against.
    ///
    /// The host carries no anchor — see the module docs — so the snapshot holds
    /// exactly one record whichever way the rule runs.
    fn render(&self, cell: &Cell, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut host = div()
            .flex()
            .flex_row()
            .items_center()
            .w(px(f32::from(cell.width)));
        if !cell.has(StateFlag::Empty) {
            host = host.h(HOST_HEIGHT);
        }
        host.child(self.separator().render(theme, anchors))
            .into_any_element()
    }
}

/// Which way the rule runs.
fn parse_orientation(raw: &str) -> Result<Orientation, ParseError> {
    ALL_ORIENTATIONS
        .into_iter()
        .find(|orientation| orientation.name() == raw)
        .ok_or_else(|| {
            ParseError::Rejected(format!(
                "--orientation takes one of {}, not {raw}",
                names(ALL_ORIENTATIONS.into_iter().map(Orientation::name)),
            ))
        })
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
    [
        (
            "--orientation <name>".to_owned(),
            format!(
                "one of {} — which way the rule runs [{}]",
                names(ALL_ORIENTATIONS.into_iter().map(Orientation::name)),
                Separator::fixture().orientation.name(),
            ),
        ),
        (
            "--call-site <name>".to_owned(),
            format!(
                "one of {} — the className bundle a call site merges; none of them pins \
                 a height, which is what keeps self-stretch live [{}]",
                names(ALL_CALL_SITES.into_iter().map(CallSite::name)),
                Separator::fixture().call_site.name(),
            ),
        ),
    ]
    .into_iter()
    .collect()
}
