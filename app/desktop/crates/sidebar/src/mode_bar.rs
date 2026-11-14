//! The Chats, Files and Changes switch.

use gpui::{AnyElement, App, Context, Entity, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{IconName, Surface, TabItem, Tabs};

use crate::focus::Focus;
use crate::label_visibility::LabelVisibility;
use crate::mode::Mode;
use crate::sidebar::Sidebar;

fn item(mode: Mode, labels: LabelVisibility, (shown, focus): (Mode, Focus)) -> TabItem {
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
    move |index, _, cx| {
        if let Some(mode) = listed.get(index).copied() {
            sidebar.update(cx, |this, cx| this.set_mode(mode, cx));
        }
    }
}

impl Sidebar {
    pub(crate) fn mode_bar(&mut self, width: f32, cx: &mut Context<'_, Self>) -> AnyElement {
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
            .px(px(8.0))
            .py(px(6.0))
            .child(tabs)
            .into_any_element()
    }
}

#[cfg(test)]
#[path = "mode_bar_tests.rs"]
mod tests;
