//! `workspace_switcher::Row` — no `--surface` exists for this content (see
//! `crowbar_ui::components::workspace_switcher`'s own module docs and
//! `native/mapping/workspace-switcher.md`), and this file is the honest
//! coverage that decision leaves on the table, the `row_layout::
//! sidebar_tab_bar` shape applied to a second "no root of its own"
//! composition.

use super::{assert_px, find, ids, lay_out};
use crowbar_driver::RawAnchor;
use crowbar_ui::Theme;
use crowbar_ui::components::autocomplete;
use crowbar_ui::components::repo_avatar::{Kind, RepoAvatar, Size};
use crowbar_ui::components::workspace_branch_icon::Status;
use crowbar_ui::components::workspace_switcher::Row;
use crowbar_ui::ui_sans_font;
use gpui::{
    Context, IntoElement, ParentElement as _, Render, SharedString, Styled as _, TestAppContext,
    Window, div, px, size,
};

use crate::driver_anchors::DriverAnchors;

/// A one-view stage around a bare [`Row`] — the same shape
/// `row_layout::sidebar_tab_bar`'s own `Stage` wraps a bare `SidebarTabBar`
/// in, minus the `Cell`.
struct Stage {
    row: Row,
    highlighted: bool,
    theme: Theme,
}

impl Render for Stage {
    fn render(&mut self, _window: &mut Window, _cx: &mut Context<Self>) -> impl IntoElement {
        div()
            .w(px(300.0))
            .font(ui_sans_font(&self.theme))
            .child(self.row.render(&self.theme, &DriverAnchors, self.highlighted))
    }
}

fn measure_row(cx: &mut TestAppContext, row: Row, highlighted: bool) -> Vec<RawAnchor> {
    let theme = Theme::DARK;
    let (_anchors, records) = lay_out(cx, size(px(400.0), px(120.0)), |_, _| Stage {
        row,
        highlighted,
        theme,
    });
    records
}

/// **`Row::render` never opts a second, fabricated root into the sink — the
/// one anchor it carries is `autocomplete::ID_ITEM`, reused.**
///
/// **Mutation:** minting `anchors.root("workspace-switcher-item", …)`
/// inside `Row::render` (the fabricated-anchor move the module docs reject)
/// turns this red twice over: a second id appears, and there is no
/// `Surface` this test's own harness could have checked it against either
/// — which is exactly why this test has to exist here instead.
#[gpui::test]
fn the_only_anchor_is_autocompletes_own_item_id(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure_row(cx, Row::fixture(), true);
    let seen = ids(&records);

    assert!(seen.contains(&autocomplete::ID_ITEM.to_owned()), "{seen:?}");
    assert!(
        !seen.iter().any(|id| id == "workspace-switcher-item"),
        "{seen:?} — no fabricated id",
    );
}

/// **The fixture's own row comes out at the content height `autocomplete::
/// Item::fixture` already documents for this call site**: `2 × 6 (py-1.5)
/// + 18 = 30px` — read off a real taffy layout rather than the reference's
/// own hand arithmetic.
///
/// **Mutation, empirically checked:** swapping `command::ITEM_PADDING_Y`
/// for `autocomplete::ITEM_PADDING_Y` (4px, not 6) in the `.h(…)` line of
/// `Row::shell` turns this red — 26px, not 30 (confirmed by running the
/// mutation). The `.py(…)` line one statement above is **not** the one to
/// mutate: it moves the row's own padding but not its explicit `.h(…)`,
/// which is authored independently rather than derived from it — mutating
/// `.py(…)` alone leaves this test green, a real, checked trap in this
/// component's own shape, not merely a hypothetical one.
#[gpui::test]
fn the_fixture_row_is_30px_tall(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let records = measure_row(cx, Row::fixture(), true);
    let item = find(&records, autocomplete::ID_ITEM);
    assert_px(item.bounds.size.height, px(30.0));
}

/// **A working, avatar-bearing workspace row shows the spinner, not the
/// avatar** — the identical precedence `context_pill.rs`'s own `row_layout`
/// test holds for the pill.
#[gpui::test]
fn the_spinner_beats_the_avatar_when_working(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let avatar = RepoAvatar {
        kind: Kind::Letter,
        size: Size::Sm,
        label: SharedString::new_static("R"),
        emoji: SharedString::new_static("\u{1f98a}"),
        background: Theme::DARK.primary,
    };
    let row = Row::Workspace {
        status: Status::New,
        working: true,
        repo_name: "repo".into(),
        branch_name: "default".into(),
        added: None,
        deleted: None,
        is_current: false,
        repo_avatar: Some(avatar),
    };

    let seen = ids(&measure_row(cx, row, true));
    assert!(seen.iter().any(|id| id == "workspace-branch-icon"), "{seen:?}");
    assert!(!seen.iter().any(|id| id == "repo-avatar"), "{seen:?}");
}

/// **`is_current` adds no anchor** — `<Check>` carries no `data-oracle-id`
/// in the source (see the module docs), so a current row's own anchor set
/// is identical to a non-current one's: `autocomplete::ID_ITEM` plus the
/// status glyph's own `workspace-branch-icon`, unchanged either way.
///
/// **Mutation:** anchoring `Row::check_icon`'s own box (opting a
/// `"workspace-switcher-check"` id in for `is_current` cells) turns this
/// red — the two sets would then differ by exactly that one id.
#[gpui::test]
fn is_current_adds_no_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let Row::Workspace {
        status,
        working,
        repo_name,
        branch_name,
        added,
        deleted,
        repo_avatar,
        ..
    } = Row::fixture()
    else {
        panic!("the fixture is a workspace row");
    };
    let not_current = Row::Workspace {
        status,
        working,
        repo_name: repo_name.clone(),
        branch_name: branch_name.clone(),
        added,
        deleted,
        is_current: false,
        repo_avatar: repo_avatar.clone(),
    };
    let current = Row::Workspace {
        status,
        working,
        repo_name,
        branch_name,
        added,
        deleted,
        is_current: true,
        repo_avatar,
    };

    let mut without = ids(&measure_row(cx, not_current, true));
    let mut with = ids(&measure_row(cx, current, true));
    without.sort();
    with.sort();
    assert_eq!(without, with);
    assert!(without.contains(&autocomplete::ID_ITEM.to_owned()));
    assert!(without.contains(&"workspace-branch-icon".to_owned()));
}

/// **A home row's own anchor set is `autocomplete::ID_ITEM` alone** —
/// `Library`/`Check` are both unanchored (no native equivalent, no
/// `data-oracle-id` in the source), unlike a workspace row's, which always
/// adds the status glyph's own anchor.
#[gpui::test]
fn a_home_row_carries_only_the_item_anchor(cx: &mut TestAppContext) {
    crowbar_driver::leak_checked!(cx);
    let home = Row::Home {
        project_name: "acme".into(),
        is_current: false,
    };
    let seen = ids(&measure_row(cx, home, true));
    assert_eq!(seen, vec![autocomplete::ID_ITEM.to_owned()]);
}
