//! `sidebar_tab_bar` — no `--surface` exists for this composition, and this
//! file is the honest coverage that decision leaves on the table.
//!
//! `crowbar_ui::components::sidebar_tab_bar`'s own module docs make the
//! ruling in full (its final section, "Why this stays a `crowbar-ui` field
//! rather than becoming its own `--surface`"): `sidebar-tab-bar.tsx`'s own
//! wrapper `<div>` carries no `data-oracle-id`, so `SidebarTabBar::render`
//! returns the wrapped `Tabs` element directly rather than opting a second,
//! fabricated root into `anchors` — and `surface.rs`'s own
//! `every_registered_surface_has_its_own_name_and_root` forbids two
//! surfaces sharing a root regardless. `native/mapping/sidebar-tab-bar.md`
//! carries the same verdict, argued fresh rather than assumed from that
//! comment.
//!
//! None of that is a reason this composition's own geometry — the `px-2`
//! arithmetic that turns a 294px column into `tabs.md`'s own captured 278px,
//! and the fact that the wrapper's padding genuinely *offsets* the nested
//! `tabs` root rather than only shrinking it — has to go unmeasured. What
//! follows drives [`SidebarTabBar::render`] directly through the shared
//! `row_layout` harness's own [`lay_out`](super::lay_out), the identical
//! instrument every registered surface's own tests use; the only thing
//! missing here is a `Cell`/`Surface` pair to select it from, because there
//! is no anchor of this wrapper's own for a `Surface::root` to name.

use super::{assert_px, find, ids, lay_out};
use crowbar_driver::RawAnchor;
use crowbar_ui::Theme;
use crowbar_ui::components::sidebar_tab_bar::SidebarTabBar;
use crowbar_ui::components::tabs;
use crowbar_ui::ui_sans_font;
use gpui::{
    Context, IntoElement, ParentElement as _, Pixels, Render, Size, Styled as _, TestAppContext,
    Window, div, px, size,
};

use crate::driver_anchors::DriverAnchors;

/// A one-view stage around a bare [`SidebarTabBar`] — the same shape
/// `row_layout.rs`'s own `Stage` wraps a `Cell` in, minus the `Cell`: this
/// composition has no surface to select one through.
struct Stage {
    bar: SidebarTabBar,
    theme: Theme,
}

impl Render for Stage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        // Mirrors `row_layout::Stage::render`'s own ambient font — every leaf
        // that paints text declares its own family explicitly, so this is
        // never what actually shapes a run, but it keeps a stray inherited
        // TOFU font off the table the same way that root does.
        div()
            .font(ui_sans_font(&self.theme))
            .child(self.bar.render(&self.theme, &DriverAnchors))
    }
}

/// Lays a bare [`SidebarTabBar`] out at the given window size and hands back
/// what the extractor recorded.
fn measure_bar(cx: &mut TestAppContext, bar: SidebarTabBar, window: Size<Pixels>) -> Vec<RawAnchor> {
    let theme = Theme::DARK;
    let (_anchors, records) = lay_out(cx, window, |_, _| Stage { bar, theme });
    records
}

/// The live sidebar tab bar, at some column width, in a window comfortably
/// wider and taller than any cell this file drives.
fn window_for(column_width: f32) -> Size<Pixels> {
    size(px(column_width + 200.0), px(120.0))
}

/// **`SidebarTabBar::render` never opts a second root into the sink.** Every
/// anchor a rendered bar carries belongs to `tabs::ID_ROOT`'s own subtree —
/// `sidebar-tab-bar` itself never appears, which is the structural claim the
/// component's own module docs make and this is what holds the port to it.
///
/// **Mutation:** wrapping the returned element in
/// `anchors.root(ID_SIDEBAR_TAB_BAR.into(), self.shell()...)` in
/// `SidebarTabBar::render` — the fabricated-anchor move the module docs
/// reject — turns this red twice over: a second id appears, and
/// `surface.rs`'s own `every_registered_surface_has_its_own_name_and_root`
/// would have nothing to say about it, because this composition still has no
/// `Surface` to check it against — which is exactly why this test has to
/// exist here instead.
#[gpui::test]
fn no_second_root_is_ever_opted_in(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let bar = SidebarTabBar::fixture();
    let records = measure_bar(cx, bar, window_for(294.0));
    let seen = ids(&records);

    assert!(
        !seen.iter().any(|id| id == "sidebar-tab-bar"),
        "{seen:?} — this wrapper must never carry an anchor of its own",
    );
    for id in [
        tabs::ID_ROOT,
        tabs::ID_LIST,
        tabs::ID_INDICATOR,
        "tabs-tab-workspaces",
        "tabs-tab-chats",
        "tabs-tab-files",
    ] {
        assert!(seen.contains(&id.to_owned()), "{id} missing from {seen:?}");
    }
    assert_eq!(seen.len(), 6, "{seen:?}");
}

/// **The fixture's own 294px column produces `tabs.md`'s own captured
/// 278px** — `294 − 2 × 8 = 278` — read off a real taffy layout rather than
/// the module docs' hand arithmetic.
///
/// **Mutation:** changing `PADDING_X` from `SPACING * 2.0` to `SPACING` in
/// `crowbar_ui::components::sidebar_tab_bar` turns this red — the root would
/// measure 286px, not 278.
#[gpui::test]
fn the_fixtures_column_reproduces_tabs_mds_own_278px(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let bar = SidebarTabBar::fixture();
    let records = measure_bar(cx, bar, window_for(294.0));

    let root = find(&records, tabs::ID_ROOT);
    assert_px(root.bounds.size.width, px(278.0));
}

/// **The `px-2 × 2 = 16px` arithmetic holds at other column widths too**, not
/// only the one captured cell — the container query and `Tabs`' own
/// `w-full` both resolve against this identical number (module docs §1–2).
#[gpui::test]
fn the_sixteen_pixel_arithmetic_holds_at_other_widths(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    for column_width in [200.0f32, 294.0, 420.0] {
        let bar = SidebarTabBar {
            column_width: px(column_width),
            ..SidebarTabBar::fixture()
        };
        let records = measure_bar(cx, bar, window_for(column_width));
        let root = find(&records, tabs::ID_ROOT);
        assert_px(root.bounds.size.width, px(column_width - 16.0));
    }
}

/// **The wrapper's own padding genuinely offsets the nested `tabs` root, not
/// only shrinks it** — the tabs root's own top-left corner sits at
/// `(PADDING_X, PADDING_Y)` = `(8, 6)` off the stage's own origin, which a
/// wrapper that only changed *width* (say, a `max-width` instead of real
/// padding) would not reproduce.
///
/// **Mutation:** replacing `.px(PADDING_X).py(PADDING_Y)` with
/// `.max_w(self.column_width - PADDING_X * 2.0)` in
/// `SidebarTabBar::shell` — a hypothetical alternative that produces the
/// identical 278px width — turns this red: the tabs root would then sit at
/// `(0, 0)`, not `(8, 6)`.
#[gpui::test]
fn the_tabs_root_sits_at_the_wrappers_own_padding(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let bar = SidebarTabBar::fixture();
    let records = measure_bar(cx, bar, window_for(294.0));

    let root = find(&records, tabs::ID_ROOT);
    assert_px(root.bounds.origin.x, px(8.0));
    assert_px(root.bounds.origin.y, px(6.0));
}

/// **`include_git` adds a fourth, real anchor** — `tabs-tab-git` — to the set
/// this wrapper's own rendered tree carries; the other three keep their own
/// ids, and the total count moves from six to seven.
#[gpui::test]
fn include_git_adds_the_fourth_tab_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);

    let without_git = SidebarTabBar::fixture();
    let with_git = SidebarTabBar {
        include_git: true,
        ..SidebarTabBar::fixture()
    };

    let seen_without = ids(&measure_bar(cx, without_git, window_for(294.0)));
    let seen_with = ids(&measure_bar(cx, with_git, window_for(294.0)));

    assert!(!seen_without.iter().any(|id| id == "tabs-tab-git"));
    assert!(seen_with.iter().any(|id| id == "tabs-tab-git"));
    assert_eq!(seen_without.len(), 6, "{seen_without:?}");
    assert_eq!(seen_with.len(), 7, "{seen_with:?}");
}
