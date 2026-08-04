//! `skeleton` — a loading placeholder, the port's **second** component with no
//! reference, and the one where `ANCHORS.md` v1.9 turns out not to bite.
//!
//! The native half of `web/src/components/ui/skeleton.tsx`: a bare `<div>` with
//! one class list, no variants and no size of its own. Every value below came
//! out of the app's own `tailwindcss` 4.3.0 with the utility as a candidate —
//! the method `native/MAPPING.md` fixes. See `native/mapping/skeleton.md`.
//!
//! # Why there is no reference: the only call site **cannot render**
//!
//! `<Skeleton>` has exactly one importer,
//! `web/src/components/layout/sidebar-skeleton.tsx`, and `SidebarSkeleton` has
//! exactly one call site:
//!
//! ```text
//! sidebar-carousel.tsx:131   <Suspense fallback={<SidebarSkeleton />}>
//!                              <FileExplorerTree … />
//! ```
//!
//! A `<Suspense>` boundary shows its fallback only while a descendant suspends.
//! `FileExplorerTree` is a **static import** with no suspending source —
//! `grep` over `web/src` finds no `React.lazy` inside that subtree, no
//! `useSuspenseQuery`, and no `use()`. Every `lazy()` in the tree is under
//! `features/panes/`, outside this boundary. So the fallback has nothing to wait
//! for and never mounts: a live count of `[data-slot="skeleton"]` was **0** in
//! every state, including immediately after a full reload.
//!
//! This is `git-row-dir`'s precedent — rendered by the port, absent from the
//! product — except that here it is absent *by construction* rather than by the
//! fixture. **Nothing below is a guess**; the values are the app's compiled CSS.
//!
//! # v1.9 does not apply, and that is a measurement rather than luck
//!
//! `animate-skeleton` compiles to `animation: skeleton 2s -1s infinite linear`
//! over `@keyframes skeleton { to { background-position: -200% 0 } }`. So the
//! element **is** animated, and v1.9's warning — that a snapshot cannot say when
//! it was taken — is the first thing to check.
//!
//! It does not reach this component, because the only property in flight is
//! `background-position`, and **no field in the contract records it**:
//!
//! * `bounds` are fixed by the call site's `h-*`/`w-*`; nothing in the keyframes
//!   touches geometry.
//! * `bg` is read by the extractor as
//!   `oracleNormalizeColor(paint.backgroundColor)` — `web/src/lib/oracle/extract.ts`
//!   — which is the **`background-color`** layer, `var(--color-muted)`. The
//!   sheen is a `background-image` gradient and is not read at all.
//!
//! So a capture of this surface is timing-independent in every recorded field,
//! and would be whether it were taken at rest or mid-sweep. That is a stronger
//! statement than "captured at rest", and it is the reason this module does not
//! carry a settling caveat.
//!
//! # The sheen is deliberately unmodelled, and is invisible to the oracle
//!
//! The port paints the resting colour and the corner; it does not
//! draw the moving gradient. That is a real visual gap — it is what makes a
//! skeleton read as *loading* — but it is one the contract cannot see, so
//! closing it would be unverifiable by the oracle either way, and it needs a
//! frame loop this leaf has no other use for.
//!
//! It is stated here rather than as a `const … : bool = true` with a test
//! asserting it: that would be a guard that tests its own declaration and not
//! any behaviour. [`Skeleton::sweep`] carries the one part of the animation that
//! *is* a value — its duration — so the number has a home if the sheen is ever
//! drawn.
//!
//! # Every live call site overrides the primitive's own radius
//!
//! `rounded-sm` is the primitive's, and all **seven** shapes in
//! `sidebar-skeleton.tsx` name their own — five `rounded`, two `rounded-md`.
//! The primitive's radius is therefore dead at every call site that exists,
//! which is the same shape of finding as `label`'s dead `ui-text-sm` and is why
//! [`CallSite`] carries the radius rather than the component defaulting it.

use gpui::{AnyElement, Div, Pixels, Styled as _, div, px};
use std::time::Duration;

use crate::anchor::{AnchorId, AnchorSink};
use crate::theme::Theme;

/// The single anchor this surface carries.
pub const ID_SKELETON: &str = "skeleton";

/// **Nothing.** A skeleton paints no text, and every live call site pins both
/// axes with `h-*` and `w-*` or stretches one with `flex-1` — none of which is
/// a content width.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** This box paints no text, so there is no line box for a height to
/// be derived from; `ANCHORS.md` v1.6 makes the declaration valid only on an
/// anchor that carries a `font`.
pub const LINE_SIZED: [&str; 0] = [];

/// `rounded` — the literal `0.25rem`, five of the seven live shapes.
pub const RADIUS_CALL_SITE: Pixels = px(4.0);

/// The `className` bundle a call site merges over the primitive's own.
///
/// Named after the shapes `sidebar-skeleton.tsx` actually builds. Every one of
/// them overrides the radius, and each pins its own box — a skeleton has no size
/// of its own at all.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CallSite {
    /// No className: the primitive alone, `rounded-sm` and no box. The only
    /// cell in which the primitive's own radius is the one painted.
    None,
    /// `h-5 w-5 rounded-md flex-shrink-0` — the row's leading icon block.
    Icon,
    /// `h-3 flex-1 rounded` — the row's flexible title bar.
    Title,
    /// `h-3 w-8 rounded` — the row's trailing meta bar.
    Meta,
}

/// Every modelled call site, for `--help` and the closed-vocabulary test.
pub const ALL_CALL_SITES: [CallSite; 4] = [
    CallSite::None,
    CallSite::Icon,
    CallSite::Title,
    CallSite::Meta,
];

impl CallSite {
    /// The word `--call-site` takes.
    #[must_use]
    pub fn name(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Icon => "icon",
            Self::Title => "title",
            Self::Meta => "meta",
        }
    }

    /// The height this call site pins, if any.
    #[must_use]
    pub fn height(self) -> Option<Pixels> {
        match self {
            Self::None => None,
            Self::Icon => Some(px(20.0)),
            Self::Title | Self::Meta => Some(px(12.0)),
        }
    }

    /// The width this call site pins, if any. `Title` pins none — it is
    /// `flex-1` and takes what the row leaves it.
    #[must_use]
    pub fn width(self) -> Option<Pixels> {
        match self {
            Self::None | Self::Title => None,
            Self::Icon => Some(px(20.0)),
            Self::Meta => Some(px(32.0)),
        }
    }

    /// Whether the call site is `flex-1`.
    #[must_use]
    pub fn grows(self) -> bool {
        matches!(self, Self::Title)
    }

    /// The corner this call site paints, or `None` to keep the primitive's
    /// `rounded-sm`.
    ///
    /// **Only [`CallSite::None`] returns `None`**, and that cell has no live
    /// existence — see the module docs.
    #[must_use]
    pub fn radius(self, theme: &Theme) -> Option<Pixels> {
        match self {
            Self::None => None,
            Self::Icon => Some(theme.radius_md.into()),
            Self::Title | Self::Meta => Some(RADIUS_CALL_SITE),
        }
    }
}

/// One `<Skeleton>`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Skeleton {
    /// The call site whose className is merged over the primitive's.
    pub call_site: CallSite,
}

impl Skeleton {
    /// The leading icon block of a `SidebarSkeleton` chat row — the shape the
    /// surface renders by default.
    ///
    /// **Not a captured fixture**, and the doc comment says so on purpose: see
    /// the module docs for why no capture exists.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            call_site: CallSite::Icon,
        }
    }

    /// How long one sweep of the sheen takes — `--animate-skeleton`'s `2s`, off
    /// the sealed theme rather than restated here.
    ///
    /// Read by nothing this port draws — the sheen is unmodelled, see the module
    /// docs — and carried so the duration has one home if it ever is.
    #[must_use]
    pub fn sweep(theme: &Theme) -> Duration {
        theme.animate_skeleton.into()
    }

    /// The placeholder's box.
    ///
    /// The paint is the `background` shorthand's **colour** layer,
    /// `var(--color-muted)` — which is also the only layer the contract records.
    fn shell(self, theme: &Theme) -> Div {
        let mut element = div().bg(theme.color_muted);

        element = match self.call_site.radius(theme) {
            Some(radius) => element.rounded(radius),
            None => element.rounded(Pixels::from(theme.radius_sm)),
        };
        if let Some(height) = self.call_site.height() {
            element = element.h(height);
        }
        if let Some(width) = self.call_site.width() {
            element = element.w(width);
        }
        if self.call_site.grows() {
            element = element.flex_grow_1();
        }
        element
    }

    /// The element, with its one anchor.
    pub fn render(self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        anchors.boxed(AnchorId::from(ID_SKELETON), self.shell(theme))
    }
}

#[cfg(test)]
mod tests {
    use super::{ALL_CALL_SITES, CallSite, RADIUS_CALL_SITE, Skeleton};
    use crate::theme::{Appearance, Theme};
    use gpui::Pixels;

    /// Neither declaration is made, and each for its own reason — see the
    /// constants.
    #[test]
    fn a_skeleton_declares_neither_content_nor_line_sizing() {
        assert!(super::CONTENT_SIZED.is_empty());
        assert!(super::LINE_SIZED.is_empty());
    }

    /// **Every live call site overrides the primitive's radius**, which is the
    /// finding this component carries. Only the `none` cell — which has no live
    /// existence — keeps `rounded-sm`.
    #[test]
    fn only_the_bare_primitive_keeps_its_own_radius() {
        let theme = Theme::for_appearance(Appearance::Dark);
        for call_site in ALL_CALL_SITES {
            let overridden = call_site.radius(&theme).is_some();
            assert_eq!(
                overridden,
                call_site != CallSite::None,
                "{}",
                call_site.name(),
            );
        }
        // And the two are genuinely different numbers, so the assertion above is
        // not one that would pass whatever the radii were.
        assert_ne!(
            CallSite::Icon.radius(&theme),
            Some(Pixels::from(theme.radius_sm)),
        );
        assert_eq!(CallSite::Meta.radius(&theme), Some(RADIUS_CALL_SITE));
    }

    /// A skeleton has **no size of its own**: the bare primitive pins neither
    /// axis, and every live shape pins at least one.
    #[test]
    fn the_primitive_pins_no_box_and_every_live_shape_does() {
        assert_eq!(CallSite::None.height(), None);
        assert_eq!(CallSite::None.width(), None);
        assert!(!CallSite::None.grows());

        for call_site in [CallSite::Icon, CallSite::Title, CallSite::Meta] {
            assert!(call_site.height().is_some(), "{}", call_site.name());
            // `Title` is the one that takes its width from the row instead.
            assert_eq!(
                call_site.width().is_none(),
                call_site.grows(),
                "{}",
                call_site.name(),
            );
        }
    }

    /// The sweep is the sealed token's 2s — read off the theme rather than
    /// restated, so a token change moves it here too.
    #[test]
    fn the_sweep_is_the_themes_two_seconds() {
        for appearance in [Appearance::Light, Appearance::Dark] {
            let theme = Theme::for_appearance(appearance);
            assert_eq!(Skeleton::sweep(&theme).as_secs(), 2, "{appearance:?}");
        }
    }

    /// The vocabulary is closed and its words are unique.
    #[test]
    fn the_call_site_vocabulary_is_closed() {
        let mut names: Vec<_> = ALL_CALL_SITES.iter().map(|c| c.name()).collect();
        names.sort_unstable();
        assert_eq!(names, ["icon", "meta", "none", "title"]);
        assert_eq!(Skeleton::fixture().call_site, CallSite::Icon);
    }
}
