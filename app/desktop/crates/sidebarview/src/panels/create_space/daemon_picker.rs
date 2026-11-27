use gpui::{App, Context, Entity, InteractiveElement, ParentElement, Styled, div, px};
use sidebar::Host;
use ui::{DropdownMenu, Icon, IconName, MenuAction, MenuItem, Palette};

use crate::column::host_icon;
use crate::sidebar::Sidebar;

const TRIGGER_GAP: f32 = 6.0;
const TRIGGER_HEIGHT: f32 = 28.0;
const CARET: f32 = 16.0;

fn daemon_item(
    (index, host): (usize, &Host),
    chosen: usize,
    sidebar: &Entity<Sidebar>,
) -> MenuItem {
    let sidebar = sidebar.clone();
    let action = MenuAction::new(host.name.clone(), move |_, cx: &mut App| {
        sidebar.update(cx, |this, cx| this.select_space_host(index, cx));
    });
    if index == chosen {
        return MenuItem::Action(action.checked());
    }
    MenuItem::Action(action)
}

pub(crate) fn daemon_picker(
    (hosts, chosen): (&[Host], usize),
    palette: &Palette,
    cx: &mut Context<'_, Sidebar>,
) -> Option<DropdownMenu> {
    let host = hosts.get(chosen)?;
    let sidebar = cx.entity();
    let items: Vec<MenuItem> = hosts
        .iter()
        .enumerate()
        .map(|entry| daemon_item(entry, chosen, &sidebar))
        .collect();
    let trigger = div()
        .id("space-host-trigger")
        .flex()
        .items_center()
        .gap(px(TRIGGER_GAP))
        .h(px(TRIGGER_HEIGHT))
        .min_w_0()
        .cursor_pointer()
        .text_color(palette.muted_foreground())
        .child(host_icon(host, (palette, palette.muted())))
        .child(div().truncate().child(host.name.clone()))
        .child(
            Icon::new(IconName::CaretUpDown)
                .size(px(CARET))
                .color(palette.muted_foreground()),
        );
    Some(DropdownMenu::new("space-host", trigger, items))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn a_picker_needs_the_chosen_host_to_exist(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoShell::rich());
        let hosts = sidebar.read_with(cx, |this, _| this.data.hosts());
        let shown = sidebar.update(cx, |_, cx| {
            let palette = ui::Theme::of(cx).palette(ui::Surface::Chrome);
            let chosen = daemon_picker((&hosts, 1), palette, cx).is_some();
            let missing = daemon_picker((&hosts, hosts.len()), palette, cx).is_some();
            (chosen, missing)
        });
        assert_eq!(shown, (true, false));
    }
}
