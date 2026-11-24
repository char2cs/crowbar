//! The Chats, Files and Changes switch.

use gpui::{AnyElement, App, Context, Entity, IntoElement, ParentElement, Styled, Window, div};
use sidebar::{Focus, Mode};
use ui::{IconName, Surface, TabItem, Tabs};

use crate::column::label_visibility::LabelVisibility;
use crate::sidebar::Sidebar;
use crate::sidebar::{SPACE_MD, SPACE_SM};

fn item(
    mode: Mode,
    labels: LabelVisibility,
    (shown, focus): (Mode, Focus),
) -> TabItem {
    let (icon, label) = match mode {
        Mode::Chats => (IconName::ChatsCircle, "Chats"),
        Mode::Files => (IconName::FolderOpen, "Files"),
        Mode::Changes => (IconName::GitBranch, "Changes"),
    };
    let tab = match (labels, mode == shown) {
        (LabelVisibility::All, _) | (LabelVisibility::ActiveOnly, true) => {
            TabItem::icon(icon).with_label(label)
        },
        (LabelVisibility::ActiveOnly | LabelVisibility::Hidden, _) => TabItem::icon(icon),
    };
    if mode.is_available(focus) {
        return tab;
    }
    tab.disabled()
}

fn selector(
    sidebar: Entity<Sidebar>,
    listed: Vec<Mode>,
) -> impl Fn(usize, &mut Window, &mut App) + 'static {
    move |index, window, cx| select(&sidebar, listed.get(index).copied(), window, cx)
}

fn select(
    sidebar: &Entity<Sidebar>,
    mode: Option<Mode>,
    window: &mut Window,
    cx: &mut App,
) {
    let Some(mode) = mode else {
        return;
    };
    sidebar.update(cx, |this, cx| {
        release_tree(this, window, cx);
        this.set_mode(mode, cx);
    });
}

fn release_tree(
    sidebar: &Sidebar,
    window: &mut Window,
    cx: &mut App,
) {
    if sidebar.files_focus.contains_focused(window, cx) {
        window.blur(cx);
    }
}

impl Sidebar {
    pub(crate) fn mode_bar(
        &mut self,
        width: f32,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let focus = self.data.focus();
        let shown = self.mode.shown(focus);
        let labels = LabelVisibility::for_sidebar(width);
        let listed: Vec<Mode> = Mode::ALL
            .into_iter()
            .filter(|mode| mode.is_listed(focus))
            .collect();
        let selected = listed.iter().position(|mode| *mode == shown).unwrap_or(0);
        let items = listed
            .iter()
            .map(|mode| item(*mode, labels, (shown, focus)))
            .collect();
        let sidebar = cx.entity();
        let tabs = Tabs::new("sidebar-modes", items, selected)
            .on(Surface::Chrome)
            .on_select(selector(sidebar, listed));
        div()
            .flex()
            .flex_none()
            .items_center()
            .px(SPACE_MD)
            .py(SPACE_SM)
            .child(tabs)
            .into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    use sidebar::Focus;

    #[gpui::test]
    fn choosing_a_mode_in_a_repository_shows_it(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Changes, cx));
        cx.run_until_parked();
        assert_eq!(sidebar.read_with(cx, |this, _| this.mode), Mode::Changes);
    }

    #[gpui::test]
    fn a_mode_the_focus_cannot_use_is_refused(cx: &mut TestAppContext) {
        let data = DemoShell::reference().with_focus(Focus::Nothing);
        let (_, sidebar, cx) = open(cx, data);
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        assert_eq!(sidebar.read_with(cx, |this, _| this.mode), Mode::Chats);
    }

    #[gpui::test]
    fn switching_views_takes_focus_from_the_files_tree_and_with_it_the_rows_highlight(
        cx: &mut TestAppContext
    ) {
        let (_, sidebar, cx) = open(cx, DemoShell::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        sidebar.update_in(cx, |this, window, cx| {
            let first = this.visible_files().remove(0).path;
            this.apply(Intent::SelectFile(first), window, cx);
        });
        cx.run_until_parked();
        let lit = |cx: &mut gpui::VisualTestContext| cx.debug_bounds("file-selected").is_some();
        assert!(
            lit(cx),
            "a clicked row is highlighted while the tree has focus"
        );
        for mode in [Mode::Chats, Mode::Files] {
            cx.update(|window, cx| select(&sidebar, Some(mode), window, cx));
            cx.run_until_parked();
        }
        assert!(
            !lit(cx),
            "develop drops the highlight once the tree loses focus"
        );
    }

    #[test]
    fn the_active_tab_alone_keeps_its_label_in_a_narrow_column() {
        let focus = Focus::Repository;
        let active = item(
            Mode::Files,
            LabelVisibility::ActiveOnly,
            (Mode::Files, focus),
        );
        let idle = item(
            Mode::Chats,
            LabelVisibility::ActiveOnly,
            (Mode::Files, focus),
        );
        assert_ne!(format!("{active:?}"), format!("{idle:?}"));
    }
}
