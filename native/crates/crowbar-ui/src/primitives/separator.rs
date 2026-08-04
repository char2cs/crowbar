//! `separator` — a one-pixel rule, and the first component in the port with
//! **no reference**, for a reason that is not "nobody got to it".
//!
//! The native half of `web/src/components/ui/separator.tsx`, a thin wrapper over
//! `@base-ui/react`'s `Separator`. Every value below came out of the app's own
//! `tailwindcss` 4.3.0 with the utility as a candidate — the method
//! `native/MAPPING.md` fixes — rather than off the class name. See
//! `native/mapping/separator.md`.
//!
//! # Why there is no reference (P3.6, measured)
//!
//! `<Separator>` has exactly **two importers** in the whole tree, and both are
//! Plate editor chrome:
//!
//! ```text
//! web/src/components/ui/toolbar.tsx       ToolbarGroup's trailing rule
//! web/src/components/ui/link-toolbar.tsx  the link editor's rules
//! ```
//!
//! `ToolbarGroup` reaches the screen only through `FloatingToolbar`, which
//! gates on Plate's *focused editor*: it reads `useEventEditorValue('focus')`
//! and hands the result to `useFloatingToolbarState`, so the toolbar is hidden
//! unless the editor under the caret is the focused one. In the automation
//! webview `document.hasFocus()` is **false and immovable** — the same
//! measurement that blocks every `:focus` cell in
//! `native/oracle/blocked/hover-and-focus-need-an-unlocked-screen.md`. A live
//! count of `[data-slot="separator"]` was **0** in every state reachable
//! without focus.
//!
//! So this component is `git-row-dir`'s situation with the sign flipped: it is
//! rendered by the port and *absent from the reachable product*. **Nothing
//! below is a guess** — the values are the app's compiled CSS, and what is
//! missing is only the confirmation a capture would have given.
//!
//! # `self-stretch` is conditional, and the condition is on the call site's class
//!
//! The vertical arm is not simply `w-px; align-self: stretch`. The class is
//!
//! ```text
//! data-[orientation=vertical]:not-[[class^='h-']]:not-[[class*='_h-']]:self-stretch
//! ```
//!
//! — it stretches **only when the call site names no height of its own**. A
//! `<Separator orientation="vertical" className="h-4" />` keeps its 4-unit
//! height and does not stretch. That is a real branch rather than a curiosity:
//! `ToolbarGroup` passes no className and therefore stretches, and modelling it
//! as an unconditional stretch would put a full-height rule where a call site
//! had asked for a short one. See [`CallSite`].

use gpui::{AnyElement, Div, Pixels, Styled as _, div, px};

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::Theme;

/// The single anchor this surface carries.
pub const ID_SEPARATOR: &str = "separator";

/// **Nothing.** A separator paints no text, and neither axis of its box is a
/// content width: the cross axis is a pinned pixel and the main axis is either
/// `w-full` or a stretch, both of which are the *parent's* measure.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing**, and not for want of looking: this box paints no text at all, so
/// there is no line box for its height to be derived from. `ANCHORS.md` v1.6
/// makes `line_sized` valid only on an anchor that carries a `font`.
pub const LINE_SIZED: [&str; 0] = [];

/// `h-px` / `w-px` — the rule's thickness on its cross axis.
///
/// Tailwind's `px` scale is a literal `1px`, **not** `calc(var(--spacing) * …)`,
/// so this is one logical pixel at every root font size.
pub const THICKNESS: Pixels = px(1.0);

/// Which way the rule runs — `separator.tsx`'s `orientation` prop, whose default
/// is `horizontal` in the wrapper rather than in `base-ui`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Orientation {
    /// `data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full`.
    Horizontal,
    /// `data-[orientation=vertical]:w-px` plus the conditional stretch.
    Vertical,
}

/// Both arms, for the surface's `--orientation` vocabulary and for the tests
/// that assert the vocabulary is closed.
pub const ALL_ORIENTATIONS: [Orientation; 2] = [Orientation::Horizontal, Orientation::Vertical];

impl Orientation {
    /// The word `--orientation` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::Horizontal => "horizontal",
            Self::Vertical => "vertical",
        }
    }
}

/// The `className` bundle a call site merges over the primitive's own.
///
/// Only the two live importers are modelled, plus the bare primitive. A bundle
/// is a parameter rather than a free string for `badge`'s reason: a call site's
/// classes are part of what the reference *is*, and inventing one produces a
/// picture no live screen shows.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// No className. The primitive's own classes and nothing else.
    None,
    /// `toolbar.tsx`'s `ToolbarGroup`: a vertical rule inside a
    /// `mx-1.5 py-0.5` wrapper, with **no className on the separator itself** —
    /// so [`Self::height`] is `None` and the vertical arm stretches.
    ToolbarGroup,
    /// `link-toolbar.tsx`'s horizontal rule, `className="my-1"`. The margin is
    /// the wrapper's business; what it means *here* is that the class does not
    /// start with `h-`, so a vertical separator carrying it would still stretch.
    LinkToolbar,
}

/// Every modelled call site, for `--help` and for the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 3] = [
    CallSite::None,
    CallSite::ToolbarGroup,
    CallSite::LinkToolbar,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::ToolbarGroup => "toolbar-group",
            Self::LinkToolbar => "link-toolbar",
        }
    }

    /// The height this call site pins on the separator itself, if any.
    ///
    /// **All three are `None`**, and that is the measurement rather than an
    /// oversight: no live call site passes a `h-*` class to `<Separator>`. The
    /// method exists because the primitive's `not-[[class^='h-']]` selector
    /// makes "does the call site name a height" a real branch, and a vocabulary
    /// that could not express it would hide the branch rather than model it.
    #[must_use]
    pub fn height(self) -> Option<Pixels> {
        match self {
            Self::None | Self::ToolbarGroup | Self::LinkToolbar => None,
        }
    }
}

/// One `<Separator>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Separator {
    /// Which way the rule runs.
    pub orientation: Orientation,
    /// The call site whose className is merged over the primitive's.
    pub call_site: CallSite,
}

impl Separator {
    /// The `ToolbarGroup` rule — the closest thing this component has to a live
    /// cell, and the one the surface defaults to.
    ///
    /// It is **not** a captured fixture, and the doc comment says so on purpose:
    /// see the module docs for why no capture exists.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            orientation: Orientation::Vertical,
            call_site: CallSite::ToolbarGroup,
        }
    }

    /// Whether the vertical arm's `self-stretch` survives the call site's class.
    ///
    /// Horizontal separators never stretch — the selector is prefixed
    /// `data-[orientation=vertical]:`.
    #[must_use]
    pub fn stretches(self) -> bool {
        self.orientation == Orientation::Vertical && self.call_site.height().is_none()
    }

    /// The rule's box.
    ///
    /// `shrink-0` is unconditional and lands on both arms. There is no radius
    /// and no border: `separator.tsx` names neither, and preflight zeroes
    /// `border` for every element, so a port that gave this a hairline would be
    /// inventing one.
    fn shell(self, theme: &Theme) -> Div {
        let mut element = div().flex_shrink_0().bg(theme.color_border);

        match self.orientation {
            Orientation::Horizontal => {
                element = element.h(THICKNESS).w_full();
            }
            Orientation::Vertical => {
                element = element.w(THICKNESS);
                if let Some(height) = self.call_site.height() {
                    element = element.h(height);
                } else {
                    element = element.self_stretch();
                }
            }
        }
        element
    }

    /// The element, with its one anchor.
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(AnchorId::from(ID_SEPARATOR), self.shell(theme))
    }
}

#[cfg(test)]
mod tests {
    use super::{ALL_CALL_SITES, ALL_ORIENTATIONS, CallSite, Orientation, Separator, THICKNESS};
    use gpui::px;

    /// The declaration lists are both empty, and each for its own reason — see
    /// the constants. Asserted rather than left implicit, because an empty list
    /// that grew by accident is exactly the silent divergence `ANCHORS.md` v1.5
    /// and v1.6 exist to prevent.
    #[test]
    fn a_separator_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// `h-px` and `w-px` are a literal pixel, not a spacing step.
    #[test]
    fn the_rule_is_one_logical_pixel_on_its_cross_axis() {
        assert_eq!(THICKNESS, px(1.0));
    }

    /// The stretch is **conditional on the orientation and on the call site's
    /// class**, which is the trap this component carries. A horizontal rule
    /// never stretches however it is classed.
    #[test]
    fn only_a_vertical_rule_with_no_call_site_height_stretches() {
        for call_site in ALL_CALL_SITES {
            let vertical = Separator {
                orientation: Orientation::Vertical,
                call_site,
            };
            assert_eq!(
                vertical.stretches(),
                call_site.height().is_none(),
                "vertical · {}",
                call_site.name(),
            );

            let horizontal = Separator {
                orientation: Orientation::Horizontal,
                call_site,
            };
            assert!(!horizontal.stretches(), "horizontal · {}", call_site.name());
        }
    }

    /// **No live call site pins a height.** The record of a measurement: the
    /// branch exists in the CSS, and nothing in the app takes it.
    #[test]
    fn no_modelled_call_site_names_a_height() {
        for call_site in ALL_CALL_SITES {
            assert_eq!(call_site.height(), None, "{}", call_site.name());
        }
    }

    /// The two vocabularies are closed and their words are unique — the same
    /// assertion every ported component carries, so a name added to one enum and
    /// not to its `ALL_*` list fails here rather than vanishing from `--help`.
    #[test]
    fn the_vocabularies_are_closed() {
        let mut orientations: Vec<_> = ALL_ORIENTATIONS.iter().map(|o| o.name()).collect();
        orientations.sort_unstable();
        assert_eq!(orientations, ["horizontal", "vertical"]);

        let mut sites: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        sites.sort_unstable();
        assert_eq!(sites, ["link-toolbar", "none", "toolbar-group"]);
    }

    /// The fixture is the `ToolbarGroup` rule, which is the cell the surface
    /// renders by default.
    #[test]
    fn the_fixture_is_the_toolbar_group_rule() {
        let fixture = Separator::fixture();
        assert_eq!(fixture.orientation, Orientation::Vertical);
        assert_eq!(fixture.call_site, CallSite::ToolbarGroup);
        assert!(fixture.stretches());
    }
}
