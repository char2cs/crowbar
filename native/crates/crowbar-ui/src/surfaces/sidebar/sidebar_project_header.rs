//! `sidebar_project_header` — the sidebar's own top bar: a toggle on the
//! leading edge, a back/forward/settings cluster on the trailing edge, and a
//! macOS traffic-light spacer that only exists on one side of one platform.
//!
//! The native half of `web/src/components/layout/sidebar-project-header.tsx`.
//! Every length below came out of the app's own `tailwindcss` 4.3.0 with the
//! utility as a candidate — the method `native/MAPPING.md` fixes. See
//! `native/mapping/sidebar-project-header.md`.
//!
//! # A reference now exists — P3.54 closed the gap this file was written against
//!
//! At the time this module was first written, `sidebar-project-header.tsx`'s
//! own `<div>` wrote no oracle attribute at all, and its four `<Button>`s all
//! inherited `button.tsx`'s own `'data-oracle-id': 'button'` **default**,
//! before `{...props}` (`button.rs`'s own module docs) — so a hypothetical
//! capture of this root would have carried the same anchor id **four
//! times**, which `ANCHORS.md` v1.8 refuses outright rather than picks
//! between. [`SidebarProjectHeader`]'s own `toggle_id`/`back_id`/
//! `forward_id`/`settings_id` fields were authored, not derived from a live
//! id, in anticipation of exactly this fix — a bet this file's own history
//! record made and P3.54 has since paid off: `sidebar-project-header.tsx` now
//! carries `data-oracle-id="sidebar-project-header"` on the wrapper and a
//! distinct id on each of the four buttons
//! (`sidebar-project-header-toggle`/`-back`/`-forward`/`-settings`), matching
//! [`ID_SIDEBAR_PROJECT_HEADER`] and [`SidebarProjectHeader::fixture`]'s own
//! four ids exactly. `web/src/lib/oracle/extract.ts`'s own
//! `oracleSurfaceScope` entry for `sidebar-project-header` records the same
//! five ids, and its own comment is why `sidebar-toggle-icon` — the toggle's
//! nested glyph, a *different*, already-separately-ported surface — is
//! deliberately not a sixth. See `native/mapping/sidebar-project-header.md`
//! and `crowbar_app::surfaces::sidebar_project_header` for the driver surface
//! this now makes possible (P3.55).
//!
//! # This composition does not call `Button::render`, and that is deliberate
//!
//! `Button::render` calls `anchors.root(id, …)` — every button is its **own**
//! frame boundary, which is right for a button measured on its own but wrong
//! for four buttons inside a bigger surface's single root: nesting four more
//! `root()` calls inside this component's own would contest which anchor
//! `native/oracle/ANCHORS.md` §4's "every bound reported relative to it"
//! means. `dialog.rs`'s and `alert_dialog.rs`'s own footers already settle
//! this the same way, one door over: their two-button rows are collapsed to
//! `div().h(content_height)`, call-site content with no per-button anchor at
//! all. This file goes one step further than collapsing, because the
//! traffic-light spacer's arithmetic and the row-reverse mirror are worth
//! testing on their own terms: each button is a plain `anchors.boxed` box,
//! sized and radiused off `button::Size::IconSm`/`button::RadiusClass::Sm`'s
//! own **public, independently-verified values** — reusing what `button.rs`
//! measured, without reusing the anchor machinery that would collide.
//!
//! # The toggle's real glyph is drawn here, but anchored elsewhere
//!
//! `<SidebarToggleIcon />` is `crowbar_ui::components::sidebar_toggle_icon`,
//! ported with its own anchor and its own surface. Those are two separable
//! things, and this file separates them: it **paints** the panel mark, and it
//! does **not** anchor it. `oracleSurfaceScope` keeps `sidebar-toggle-icon`
//! out of this surface's capture, and `row_layout::sidebar_project_header`
//! asserts exactly that — so the artwork goes in through `button_box`'s plain
//! glyph slot rather than through `SidebarToggleIcon::render`, which would
//! anchor it.
//!
//! It was left unpainted until S1a, on the reasoning that drawing a
//! substitute would give the oracle a shape to converge on. That reasoning
//! held only while there was no real artwork: `crate::icon` vendors the app's
//! own mark, so what is drawn here is the reference's shape, not a stand-in.
//!
//! # `IS_MAC` is unconditionally true on the one platform this ships to
//!
//! `keybinding::Platform`'s own module docs already establish
//! `web/src/utils/platform.ts`'s `detectPlatform` as always returning
//! `'macos'` in a running webview. [`HEIGHT_OTHER`]/[`Platform::Other`] are
//! modelled because the source branches on them, not because either is
//! reachable — the same posture `keybinding.rs` already takes.

use gpui::{
    AnyElement, Div, InteractiveElement as _, IntoElement as _, ParentElement as _, Pixels,
    SharedString, Styled as _, div, px,
};

use crate::anchor::{AnchorId, AnchorSink};
use crate::icon::IconName;
use crate::primitives::button;
use crate::primitives::keybinding::Platform;
use crate::surfaces::rows::git_status_row::Breakpoint;
use crate::theme::{Color, Theme};

/// The single anchor this surface's own wrapper carries. The four buttons
/// inside it carry their own — see [`SidebarProjectHeader`]'s fields.
pub const ID_SIDEBAR_PROJECT_HEADER: &str = "sidebar-project-header";

/// **Nothing.** The header authors its own height and takes its width from
/// its call site; nothing here sizes to a text run.
pub const CONTENT_SIZED: [&str; 0] = [];

/// **Nothing.** The header paints no text of its own — its four children are
/// icon-only buttons.
pub const LINE_SIZED: [&str; 0] = [];

/// `--spacing`, Tailwind's stock `0.25rem` at a 16px root.
const SPACING: f32 = 4.0;

/// `h-[44px]` — a literal, not a spacing multiple; `IS_MAC`'s own arm, and the
/// only one a running webview ever produces.
pub const HEIGHT_MAC: Pixels = px(44.0);

/// `h-[34px]` — the non-mac arm. Modelled because the source branches on it;
/// no live cell reaches it.
pub const HEIGHT_OTHER: Pixels = px(34.0);

/// `w-[72px]` on the traffic-light spacer — a literal, not a spacing
/// multiple. Room for macOS's own window controls.
pub const TRAFFIC_LIGHTS_WIDTH: Pixels = px(72.0);

/// `gap-1` between the toggle, the flexible spacer and the cluster.
pub const GAP: Pixels = px(SPACING);

/// `gap-0.5` between the three cluster buttons.
pub const CLUSTER_GAP: Pixels = px(SPACING * 0.5);

/// `pl-3`/`pr-3` — the 12px breathing room against the *outer*, screen-edge
/// side.
pub const PADDING_OUTER: Pixels = px(SPACING * 3.0);

/// `pr-2`/`pl-2` — the 8px inset against the *inner* side, matching the
/// context pill's and tab bar's own inset.
pub const PADDING_INNER: Pixels = px(SPACING * 2.0);

/// The header.
#[derive(Clone, Debug, PartialEq)]
pub struct SidebarProjectHeader {
    /// `sidebarPosition === 'right'` — mirrors the whole bar and swaps which
    /// edge gets [`PADDING_OUTER`].
    pub is_right: bool,
    /// Which platform's height/spacer arm — see the module docs for why only
    /// [`Platform::Mac`] is live.
    pub platform: Platform,
    /// `canGoBack` — the back button's `disabled` prop, inverted.
    pub can_go_back: bool,
    /// `canGoForward` — the forward button's `disabled` prop, inverted.
    pub can_go_forward: bool,
    /// The toggle's own anchor id, authored — see the module docs.
    pub toggle_id: SharedString,
    /// The back button's own anchor id.
    pub back_id: SharedString,
    /// The forward button's own anchor id.
    pub forward_id: SharedString,
    /// The settings button's own anchor id.
    pub settings_id: SharedString,
}

impl SidebarProjectHeader {
    /// The live header on its resting, left-docked, mid-history cell: both
    /// jump directions available, macOS, sidebar on the left.
    #[must_use]
    pub fn fixture() -> Self {
        Self {
            is_right: false,
            platform: Platform::Mac,
            can_go_back: true,
            can_go_forward: true,
            toggle_id: "sidebar-project-header-toggle".into(),
            back_id: "sidebar-project-header-back".into(),
            forward_id: "sidebar-project-header-forward".into(),
            settings_id: "sidebar-project-header-settings".into(),
        }
    }

    /// Whether the traffic-light spacer renders at all: `IS_MAC && !isRight`.
    #[must_use]
    pub fn shows_traffic_lights(&self) -> bool {
        self.platform.is_mac() && !self.is_right
    }

    /// The bar's own height.
    #[must_use]
    pub const fn height(&self) -> Pixels {
        match self.platform {
            Platform::Mac => HEIGHT_MAC,
            Platform::Other => HEIGHT_OTHER,
        }
    }

    /// One of the four buttons' own box: `variant="ghost" size="icon-sm"
    /// rounded-sm`, empty (icon-only, unpainted glyph — see the module docs),
    /// dimmed at [`button::DISABLED_OPACITY`] when disabled.
    ///
    /// Reuses `button::Size::IconSm::extent`, `button::RadiusClass::Sm::value`
    /// and `button::BORDER_WIDTH` directly — the same numbers `Button::fixture`
    /// itself is built from, without going through `Button::render`'s own
    /// anchor machinery (see the module docs for why).
    /// The glyph inside a header button.
    ///
    /// 16px in a 28px button — the size the captured reference reports for
    /// `sidebar-toggle-icon`, which is the one header glyph the React app
    /// anchors and therefore the one whose size is measured rather than
    /// inferred.
    const GLYPH: Pixels = px(16.0);

    fn button_box(theme: &Theme, disabled: bool, glyph: Option<IconName>) -> Div {
        let extent = button::Size::IconSm.extent(Breakpoint::Sm);
        let mut element = div()
            .flex_shrink_0()
            .w(extent)
            .h(extent)
            .rounded(button::RadiusClass::Sm.value(theme))
            .border(button::BORDER_WIDTH)
            .border_color(Color::TRANSPARENT.value());
        if disabled {
            element = element.opacity(button::DISABLED_OPACITY);
        } else {
            // `variant="ghost"` is `border-transparent text-foreground
            // hover:bg-accent`. `button.rs` recorded hover as having "no
            // reference: synthetic pointer events are denied on this project's
            // machines" and left it out — which is why these four were the one
            // cluster with no hover at all. There is an instrument now
            // (`screenshot.rs`'s `hovering_a_row_changes_what_is_painted`), so
            // the rule goes in.
            element = element
                .hover(|style| style.bg(theme.accent))
                .cursor_pointer();
        }
        if let Some(glyph) = glyph {
            element = element
                .flex()
                .items_center()
                .justify_center()
                .child(glyph.render(Self::GLYPH, theme.muted_foreground));
        }
        element
    }

    /// The bar's own box.
    fn shell(&self) -> Div {
        let element = div()
            .flex()
            .w(gpui::relative(1.0))
            .flex_shrink_0()
            .items_center()
            .gap(GAP)
            .h(self.height());
        if self.is_right {
            element
                .pr(PADDING_OUTER)
                .pl(PADDING_INNER)
                .flex_row_reverse()
        } else {
            element.pl(PADDING_OUTER).pr(PADDING_INNER)
        }
    }

    /// The three-button cluster: back, forward, settings, in that DOM order
    /// regardless of `is_right` — the mirror comes from [`Self::shell`]'s own
    /// `flex_row_reverse`, not from reordering these three among themselves.
    fn cluster(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        div()
            .flex()
            .flex_shrink_0()
            .items_center()
            .gap(CLUSTER_GAP)
            .child(anchors.boxed(
                AnchorId::new(self.back_id.clone()),
                Self::button_box(theme, !self.can_go_back, Some(IconName::ArrowLeft)),
            ))
            .child(anchors.boxed(
                AnchorId::new(self.forward_id.clone()),
                Self::button_box(theme, !self.can_go_forward, Some(IconName::ArrowRight)),
            ))
            .child(anchors.boxed(
                AnchorId::new(self.settings_id.clone()),
                Self::button_box(theme, false, Some(IconName::Settings)),
            ))
            .into_any_element()
    }

    /// Renders the header, opting every contract anchor into `anchors`.
    pub fn render(&self, theme: &Theme, anchors: &dyn AnchorSink) -> AnyElement {
        let mut children: Vec<AnyElement> = Vec::new();

        if self.shows_traffic_lights() {
            children.push(
                div()
                    .flex_shrink_0()
                    .w(TRAFFIC_LIGHTS_WIDTH)
                    .into_any_element(),
            );
        }
        children.push(anchors.boxed(
            AnchorId::new(self.toggle_id.clone()),
            // The panel mark is DRAWN here but deliberately NOT anchored: it
            // is `sidebar-toggle-icon`, a separately registered surface, and
            // `oracleSurfaceScope` keeps it out of this surface's own capture
            // — `row_layout::sidebar_project_header` asserts exactly that.
            // So the artwork goes in through the plain glyph slot rather than
            // through `SidebarToggleIcon::render`, which would anchor it.
            Self::button_box(theme, false, Some(IconName::SidebarToggle)),
        ));
        children.push(div().flex_grow_1().into_any_element());
        children.push(self.cluster(theme, anchors));

        anchors.root(
            ID_SIDEBAR_PROJECT_HEADER.into(),
            self.shell().children(children),
        )
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CLUSTER_GAP, GAP, HEIGHT_MAC, HEIGHT_OTHER, PADDING_INNER, PADDING_OUTER,
        SidebarProjectHeader, TRAFFIC_LIGHTS_WIDTH,
    };
    use crate::primitives::keybinding::Platform;
    use gpui::px;

    #[test]
    fn every_length_is_the_compiled_spacing_multiple() {
        const STEP: f32 = 4.0;
        assert_eq!(GAP, px(STEP)); // gap-1
        assert_eq!(CLUSTER_GAP, px(STEP * 0.5)); // gap-0.5
        assert_eq!(PADDING_OUTER, px(STEP * 3.0)); // pl-3 / pr-3
        assert_eq!(PADDING_INNER, px(STEP * 2.0)); // pr-2 / pl-2
        // The two literals, not spacing multiples.
        assert_eq!(HEIGHT_MAC, px(44.0));
        assert_eq!(HEIGHT_OTHER, px(34.0));
        assert_eq!(TRAFFIC_LIGHTS_WIDTH, px(72.0));
        assert_ne!(HEIGHT_MAC, HEIGHT_OTHER);
    }

    /// `IS_MAC && !isRight` is the traffic-light spacer's whole condition —
    /// checked as the conjunction it is, not as two separate booleans that
    /// happen to agree on the fixture.
    #[test]
    fn the_traffic_lights_need_both_mac_and_left_docked() {
        let mac_left = SidebarProjectHeader::fixture();
        assert_eq!(mac_left.platform, Platform::Mac);
        assert!(!mac_left.is_right);
        assert!(mac_left.shows_traffic_lights());

        let mac_right = SidebarProjectHeader {
            is_right: true,
            ..SidebarProjectHeader::fixture()
        };
        assert!(!mac_right.shows_traffic_lights());

        let other_left = SidebarProjectHeader {
            platform: Platform::Other,
            ..SidebarProjectHeader::fixture()
        };
        assert!(!other_left.shows_traffic_lights());

        let other_right = SidebarProjectHeader {
            platform: Platform::Other,
            is_right: true,
            ..SidebarProjectHeader::fixture()
        };
        assert!(!other_right.shows_traffic_lights());
    }

    #[test]
    fn the_height_follows_the_platform() {
        let mac = SidebarProjectHeader::fixture();
        assert_eq!(mac.height(), HEIGHT_MAC);

        let other = SidebarProjectHeader {
            platform: Platform::Other,
            ..SidebarProjectHeader::fixture()
        };
        assert_eq!(other.height(), HEIGHT_OTHER);
    }

    #[test]
    fn the_fixture_is_the_resting_left_docked_macos_cell() {
        let header = SidebarProjectHeader::fixture();
        assert!(!header.is_right);
        assert_eq!(header.platform, Platform::Mac);
        assert!(header.can_go_back);
        assert!(header.can_go_forward);
        // Four distinct, non-empty ids — the whole reason this composition
        // can be tested at all despite carrying no reference of its own.
        let ids = [
            header.toggle_id.clone(),
            header.back_id.clone(),
            header.forward_id.clone(),
            header.settings_id.clone(),
        ];
        for id in &ids {
            assert!(!id.is_empty());
        }
        let mut sorted = ids.to_vec();
        sorted.sort();
        let before = sorted.len();
        sorted.dedup();
        assert_eq!(sorted.len(), before, "{ids:?}");
    }
}
