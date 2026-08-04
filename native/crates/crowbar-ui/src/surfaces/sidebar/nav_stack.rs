//! `nav_stack` — the sidebar's own push/pop screen stack: a base layer that
//! recedes when a screen is pushed, and the pushed screen's own back-button
//! header sitting where `SidebarProjectHeader` normally does.
//!
//! The native half of `web/src/components/layout/nav-stack.tsx`. In the real
//! app this wraps `sidebar-carousel.tsx`'s own scroll track as its
//! `children` — `sidebar_carousel.rs`'s own module docs already record that
//! the carousel port "does not render [`NavStack`] at all"; this item is
//! what fills that gap in. See `native/mapping/nav-stack.md`.
//!
//! # Re-deriving the cluster's own reasoning, not inheriting it
//!
//! `native/mapping/layout-denominator.md` §4 groups this component with
//! `sidebar-peek` as the tier's two judgment calls, on the strength of
//! `sidebar-carousel`'s own precedent: a store-driven wrapper whose
//! CSS-transition **end states** are the port target, and whose *transition
//! itself* is out of the oracle's reach. Checked against this file rather
//! than assumed: `useSidebarNavStore`'s `push`/`pop` drive a resting state
//! reachable by calling the store directly, exactly the way `sidebar-
//! carousel` is driven by `setActiveTab`, and every one of `nav-
//! stack.tsx`'s visual states — the base layer resting, the base layer
//! receded, a pushed screen showing — is a `transition-[transform,opacity]`
//! **end** state, never a value gpui would have to animate through.
//! `ANCHORS.md` §6 ("a snapshot is one instant") already puts the transition
//! itself beyond the contract; nothing here asks it to reach further than
//! `sidebar-carousel` already did. The reasoning holds for this file
//! without qualification.
//!
//! # An unbounded stack, a bounded contract: only the top screen is anchored
//!
//! `stack.map` renders one `<div>` per pushed screen, however deep the stack
//! is — but `ANCHORS.md` v1.8 refuses two anchors sharing one id, and a
//! synthetic per-depth id would need an arbitrary cap this component's own
//! store does not have. A cap would buy nothing anyway:
//! `web/src/lib/utils.ts`'s `cn = (...i) => twMerge(clsx(i))` resolves
//! Tailwind's conflicting-utility groups by keeping the *last* class in the
//! merge group, and `-translate-x-1/4` and `translate-x-full` are both
//! members of the `translate-x` group — so the later `translate-x-full`
//! (appended only when `!isTop`) always wins outright, leaving every
//! non-top screen at `opacity-0 pointer-events-none translate-x-full`
//! regardless of depth. A screen two deep and a screen five deep paint
//! pixel-identical boxes, fully clipped by the root's own `overflow-hidden`
//! — the same clipping argument `sidebar_carousel.rs` already made about a
//! snapped-out panel — so a second anchor there could never discriminate a
//! correct port from a broken one; it would only ever compare a box against
//! its own twin. `nav-stack.tsx`'s own `data-oracle-id` is therefore
//! written **conditionally**, `isTop ? 'nav-stack-screen' : undefined` — at
//! most one screen ever carries it — and this component follows suit:
//! [`NavStack`] models `top: Option<Screen>`, not a stack.
//!
//! # The base layer's own recede is modelled with a margin, not a transform
//!
//! `sidebar_carousel.rs`'s own module docs record the identical trick for
//! `scrollLeft`: gpui has no CSS `transform`, so a percentage the DOM
//! resolves through `translate()` against the element's own border box is
//! reproduced here as a negative leading margin instead. That is
//! numerically the same box here: the base layer has no authored width, so
//! its own border box and its containing block's width are the one
//! quantity both `-translate-x-1/4` and `margin-left: -25%` resolve
//! against. See [`RECEDE_FRACTION`].
//!
//! # `children` is opaque, the call `sidebar-carousel`'s own panels already made
//!
//! `NavStack`'s `children` **is** `sidebar-carousel.tsx`'s own scroll track
//! — an already-ported subtree this component does not own and must not
//! repaint a second copy of. [`NavStack::content_width`] exists for the
//! identical reason `sidebar_carousel::SidebarCarousel::panel_content_width`
//! does: the base layer's box does not depend on what is inside it (`flex`
//! cross-axis stretch sizes it off the container, not its content), and a
//! drivable filler turns that claim into a measurement. The same filler
//! stands in for `screen.component` inside a pushed screen's own body,
//! which is exactly as opaque — arbitrary content this port has no way to
//! know about generically.
//!
//! # The title's own line height is left undeclared — no live reference exists
//!
//! `text-[13px] font-semibold text-foreground` carries no paired
//! `line-height` utility, so its box is CSS `normal` — resolved through the
//! *ambient* font's own metrics table, not a number Tailwind's compiled CSS
//! states. `context_pill.rs` had the identical shape and could transfer a
//! known 18px measurement because its own `text-[13px]` run shares a font
//! (`font-editor`/`font-mono`, both `var(--editor-font-family)`) with an
//! already-captured reference. This title has no such donor: it paints
//! under the *ambient* font, not `font-mono`, and this item captures no
//! reference of its own (hard constraint in the brief). Declaring
//! `line_sized` here would pin a height this item cannot honestly derive —
//! `ANCHORS.md` v1.6 warns exactly against that — so [`ID_TITLE`] declares
//! neither, and the title's painted height is left to gpui's own default
//! text layout. A future item with a live capture may find a `bounds.h`
//! delta on this one anchor; this is why, recorded here rather than left to
//! be rediscovered as a mystery.

use gpui::{
    AnyElement, Div, FontWeight, IntoElement as _, ParentElement as _, Pixels, SharedString,
    Styled as _, div, px, relative,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::primitives::button;
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::primitives::keybinding::Platform;
use super::sidebar_project_header;
use crate::theme::{Color, Theme};

/// The outer wrapper's own anchor — every other bound on this surface is
/// reported relative to it.
pub const ID_ROOT: &str = "nav-stack";
/// The base/children layer — always present, whether resting or receded.
pub const ID_BASE: &str = "nav-stack-base";
/// The top pushed screen's own wrapper. Present only when [`NavStack::top`]
/// is `Some` — see the module docs for why no other depth ever carries an
/// id of its own.
pub const ID_SCREEN: &str = "nav-stack-screen";
/// The pushed screen's own header bar.
pub const ID_HEADER: &str = "nav-stack-header";
/// The header's back button.
pub const ID_BACK: &str = "nav-stack-back";
/// The header's title text.
pub const ID_TITLE: &str = "nav-stack-title";
/// The pushed screen's own content area, wrapping the opaque
/// `screen.component`.
pub const ID_BODY: &str = "nav-stack-body";

/// **Empty.** Every box here is authored (`flex`/`h-full`/padding) or, for
/// the title, constrained by `flex-1` rather than sized to its own content —
/// see the module docs for why the title is not on this list either.
pub const CONTENT_SIZED: [&str; 0] = [];
/// **Empty.** See the module docs' account of [`ID_TITLE`]: it *is*
/// line-sized in the true CSS sense, but with no live reference to derive
/// the number from, declaring it would be a claim this item cannot back.
pub const LINE_SIZED: [&str; 0] = [];

/// `-translate-x-1/4`, as the fraction [`gpui::relative`] takes for
/// [`Styled::ml`] — see the module docs for why a margin is the reproduction
/// rather than a transform.
pub const RECEDE_FRACTION: f32 = -0.25;

/// `gap-2` on the header — `calc(var(--spacing) * 2)` at the stock
/// `--spacing: 0.25rem`.
pub const HEADER_GAP: Pixels = px(8.0);
/// `px-3` on the header, both edges alike — unlike `SidebarProjectHeader`,
/// this header has no `is_right`-driven asymmetry.
pub const HEADER_PADDING_X: Pixels = px(12.0);
/// `border-b border-border` — Tailwind's default border width.
pub const HEADER_BORDER_WIDTH: Pixels = px(1.0);

/// A screen the stack has pushed.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Screen {
    /// `screen.title`, painted in the header.
    pub title: SharedString,
}

/// The nav stack, at one resting state.
#[derive(Clone, Debug, PartialEq)]
pub struct NavStack {
    /// The stack's own top screen, if any — see the module docs for why
    /// only ever one, never a `Vec`.
    pub top: Option<Screen>,
    /// `sidebarPosition === 'right'` — gates the traffic-light spacer the
    /// same way it does on [`sidebar_project_header::SidebarProjectHeader`].
    pub is_right: bool,
    /// Which platform's header height/spacer arm — see
    /// `sidebar_project_header`'s own module docs for why only
    /// [`Platform::Mac`] is live in a running webview.
    pub platform: Platform,
    /// The max-content width of a filler box placed inside whichever of
    /// [`ID_BASE`]/[`ID_BODY`] is currently rendered — see the module docs.
    pub content_width: Pixels,
}

impl Default for NavStack {
    fn default() -> Self {
        Self {
            // `getInitialState()`-equivalent: `useSidebarNavStore`'s own
            // `stack: []`.
            top: None,
            is_right: false,
            platform: Platform::Mac,
            content_width: px(0.0),
        }
    }
}

impl NavStack {
    /// The live resting cell: no screen pushed, macOS, sidebar on the left.
    #[must_use]
    pub fn fixture() -> Self {
        Self::default()
    }

    /// A cell with one screen pushed.
    #[must_use]
    pub fn showing(title: impl Into<SharedString>) -> Self {
        Self {
            top: Some(Screen {
                title: title.into(),
            }),
            ..Self::default()
        }
    }

    /// Whether the traffic-light spacer renders: `IS_MAC && !isRight` — the
    /// identical gate `SidebarProjectHeader::shows_traffic_lights` takes.
    #[must_use]
    pub fn shows_traffic_lights(&self) -> bool {
        self.platform.is_mac() && !self.is_right
    }

    /// The header's own height — reused, not re-derived: `nav-stack.tsx`'s
    /// own comment ties it directly to `SidebarProjectHeader`'s.
    #[must_use]
    pub const fn header_height(&self) -> Pixels {
        match self.platform {
            Platform::Mac => sidebar_project_header::HEIGHT_MAC,
            Platform::Other => sidebar_project_header::HEIGHT_OTHER,
        }
    }

    /// The outer wrapper: `relative flex min-h-0 flex-1 flex-col
    /// overflow-hidden`. `.relative()` is load-bearing — it is what makes
    /// the pushed screen's own `.absolute()` box resolve against *this* box
    /// rather than against whatever this surface happens to be measured
    /// inside.
    fn root_shell() -> Div {
        div()
            .relative()
            .flex()
            .flex_col()
            .flex_1()
            .min_h(px(0.0))
            .overflow_hidden()
    }

    /// The base/children layer: `flex h-full flex-col`, receded by
    /// [`RECEDE_FRACTION`] whenever a screen is on top.
    fn base_shell(&self) -> Div {
        let mut element = div().flex().flex_col().h_full();
        if self.top.is_some() {
            element = element.ml(relative(RECEDE_FRACTION));
        }
        element
    }

    /// The filler that stands in for opaque call-site content — see the
    /// module docs.
    fn filler(&self) -> Option<Div> {
        (self.content_width > px(0.0)).then(|| div().w(self.content_width))
    }

    /// One of the header's two buttons' own box — `variant="ghost"
    /// size="icon-sm"`, unpainted glyph (no native `ChevronLeft`, the same
    /// call `sidebar_project_header.rs`'s own buttons make).
    fn back_box(theme: &Theme) -> Div {
        let extent = button::Size::IconSm.extent(Breakpoint::Sm);
        div()
            .flex_shrink_0()
            .w(extent)
            .h(extent)
            .rounded(button::RadiusClass::Sm.value(theme))
            .border(button::BORDER_WIDTH)
            .border_color(Color::TRANSPARENT)
    }

    /// The header bar: `flex w-full flex-shrink-0 items-center gap-2
    /// border-b border-border px-3`, at [`Self::header_height`].
    fn header_shell(&self, theme: &Theme) -> Div {
        div()
            .flex()
            .w_full()
            .flex_shrink_0()
            .items_center()
            .gap(HEADER_GAP)
            .border_b(HEADER_BORDER_WIDTH)
            .border_color(theme.border)
            .px(HEADER_PADDING_X)
            .h(self.header_height())
    }

    /// The title span: `min-w-0 flex-1 truncate text-[13px] font-semibold
    /// text-foreground`. Constrained by `flex-1`, not sized to its own
    /// content — see the module docs for why this is not `content_sized`.
    fn title_shell(theme: &Theme) -> Div {
        div()
            .min_w(px(0.0))
            .flex_1()
            .overflow_hidden()
            .text_size(px(13.0))
            .font_weight(FontWeight::SEMIBOLD)
            .text_color(theme.foreground)
    }

    /// The pushed screen's own body: `flex flex-1 flex-col overflow-hidden`,
    /// wrapping the opaque `screen.component`.
    fn body_shell() -> Div {
        div().flex().flex_1().flex_col().overflow_hidden()
    }

    /// Renders the pushed screen, opting [`ID_SCREEN`], [`ID_HEADER`],
    /// [`ID_BACK`], [`ID_TITLE`] and [`ID_BODY`] into `anchors`.
    fn render_screen(
        &self,
        screen: &Screen,
        theme: &Theme,
        anchors: &dyn AnchorSink,
    ) -> AnyElement {
        let mut header_children: Vec<AnyElement> = Vec::new();
        if self.shows_traffic_lights() {
            header_children.push(
                div()
                    .flex_shrink_0()
                    .w(sidebar_project_header::TRAFFIC_LIGHTS_WIDTH)
                    .into_any_element(),
            );
        }
        header_children.push(anchors.boxed(AnchorId::from(ID_BACK), Self::back_box(theme)));
        header_children.push(anchors.boxed_text(
            AnchorId::from(ID_TITLE),
            Self::title_shell(theme),
            screen.title.clone(),
        ));

        let header = anchors.boxed(
            AnchorId::from(ID_HEADER),
            self.header_shell(theme).children(header_children),
        );

        let mut body = Self::body_shell();
        if let Some(filler) = self.filler() {
            body = body.child(filler);
        }
        let body = anchors.boxed(AnchorId::from(ID_BODY), body);

        anchors.boxed(
            AnchorId::from(ID_SCREEN),
            div()
                .absolute()
                .inset(px(0.0))
                .flex()
                .flex_col()
                .child(header)
                .child(body),
        )
    }

    /// Renders the stack, opting every contract anchor into `anchors`.
    #[must_use]
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut base = self.base_shell();
        if let Some(filler) = self.filler() {
            base = base.child(filler);
        }
        let mut children: Vec<AnyElement> = vec![anchors.boxed(AnchorId::from(ID_BASE), base)];

        if let Some(screen) = &self.top {
            children.push(self.render_screen(screen, theme, anchors));
        }

        anchors.root(
            AnchorId::from(ID_ROOT),
            Self::root_shell().children(children),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CONTENT_SIZED, HEADER_BORDER_WIDTH, HEADER_GAP, HEADER_PADDING_X, ID_BACK, ID_BASE,
        ID_BODY, ID_HEADER, ID_ROOT, ID_SCREEN, ID_TITLE, LINE_SIZED, NavStack, RECEDE_FRACTION,
        Screen,
    };
    use crate::primitives::keybinding::Platform;
    use gpui::px;

    #[test]
    fn every_length_is_the_measured_value() {
        assert_eq!(HEADER_GAP, px(8.0)); // gap-2
        assert_eq!(HEADER_PADDING_X, px(12.0)); // px-3
        assert_eq!(HEADER_BORDER_WIDTH, px(1.0));
        assert!((RECEDE_FRACTION + 0.25).abs() < f32::EPSILON);
    }

    #[test]
    fn neither_declaration_list_has_an_entry() {
        assert!(CONTENT_SIZED.is_empty());
        assert!(LINE_SIZED.is_empty());
    }

    #[test]
    fn the_fixture_is_the_resting_empty_stack() {
        let stack = NavStack::fixture();
        assert!(stack.top.is_none());
        assert!(!stack.is_right);
        assert_eq!(stack.platform, Platform::Mac);
        assert_eq!(stack.content_width, px(0.0));
        assert!(stack.shows_traffic_lights());
    }

    #[test]
    fn showing_pushes_exactly_one_titled_screen() {
        let stack = NavStack::showing("Projects");
        assert_eq!(
            stack.top,
            Some(Screen {
                title: "Projects".into()
            }),
        );
    }

    /// `IS_MAC && !isRight` is the traffic-light spacer's whole condition,
    /// checked as the conjunction it is.
    #[test]
    fn the_traffic_lights_need_both_mac_and_left_docked() {
        let mac_left = NavStack::fixture();
        assert!(mac_left.shows_traffic_lights());

        let mac_right = NavStack {
            is_right: true,
            ..NavStack::fixture()
        };
        assert!(!mac_right.shows_traffic_lights());

        let other_left = NavStack {
            platform: Platform::Other,
            ..NavStack::fixture()
        };
        assert!(!other_left.shows_traffic_lights());
    }

    /// The header height is reused from `sidebar_project_header`, not
    /// re-derived — the two constants are the same values by construction,
    /// asserted here so a future edit to either cannot drift silently.
    #[test]
    fn the_header_height_matches_sidebar_project_headers_own() {
        use crate::surfaces::sidebar::sidebar_project_header::{HEIGHT_MAC, HEIGHT_OTHER};

        assert_eq!(NavStack::fixture().header_height(), HEIGHT_MAC);
        let other = NavStack {
            platform: Platform::Other,
            ..NavStack::fixture()
        };
        assert_eq!(other.header_height(), HEIGHT_OTHER);
        assert_ne!(HEIGHT_MAC, HEIGHT_OTHER);
    }

    /// Every id is distinct and namespaced under this surface's own prefix.
    #[test]
    fn every_anchor_id_is_distinct_and_namespaced() {
        let ids = [
            ID_ROOT, ID_BASE, ID_SCREEN, ID_HEADER, ID_BACK, ID_TITLE, ID_BODY,
        ];
        let mut sorted = ids.to_vec();
        sorted.sort_unstable();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
        for id in ids {
            assert!(id.starts_with("nav-stack"), "{id}");
        }
    }
}
