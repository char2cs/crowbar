//! The 42px header: settings, host picker and the sidebar toggle.

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, Context, InteractiveElement, IntoElement, ParentElement, Styled, WindowControlArea,
    div, px,
};
use ui::{IconName, Surface, Theme};

use crate::command::Command;
use crate::dock_side::DockSide;
use crate::host::Host;
use crate::host_picker::host_picker;
use crate::icon_button::IconButton;
use crate::intent::Intent;
use crate::metrics::{HEADER_HEIGHT, TRAFFIC_LIGHT_RESERVE};
use crate::row_action::RowAction;
use crate::scale::{SPACE_LG, SPACE_MD, SPACE_XS};
use crate::sidebar::Sidebar;
use crate::visibility::Visibility;

impl Sidebar {
    pub(crate) fn header(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let side = self.panel.side();
        let picker = self.host_choice(cx);
        div()
            .flex()
            .flex_none()
            .items_center()
            .gap(SPACE_XS)
            .h(px(HEADER_HEIGHT))
            .w_full()
            .overflow_hidden()
            .window_control_area(WindowControlArea::Drag)
            .when(side == DockSide::Left, |d| d.pl(SPACE_LG).pr(SPACE_MD))
            .when(side == DockSide::Right, |d| {
                d.flex_row_reverse().pr(SPACE_LG).pl(SPACE_MD)
            })
            .child(div().flex_none().w(px(traffic_light_reserve(side))))
            .child(Self::settings_button(cx))
            .child(div().flex().flex_1().justify_center().children(picker))
            .child(self.toggle_button(cx))
            .into_any_element()
    }

    fn settings_button(cx: &mut Context<'_, Self>) -> IconButton {
        let action = RowAction::Intent(Intent::Send(Command::OpenSettings));
        IconButton::new("sidebar-settings", IconName::Gear)
            .tooltip("Settings")
            .on_click(cx.listener(move |this, _, window, cx| {
                this.act(action.clone(), window, cx);
            }))
    }

    fn toggle_button(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> IconButton {
        let icon = match self.panel.side() {
            DockSide::Left => IconName::SidebarToggle,
            DockSide::Right => IconName::SidebarToggleRight,
        };
        let tooltip = match self.panel.visibility() {
            Visibility::Open => "Hide Sidebar",
            Visibility::Closed => "Show Sidebar",
        };
        IconButton::new("sidebar-toggle", icon)
            .tooltip(tooltip)
            .on_click(cx.listener(|this, _, _, cx| this.toggle(cx)))
    }

    fn host_choice(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Option<AnyElement> {
        let hosts = self.data.hosts();
        let current = self.current_host(&hosts)?;
        if hosts.len() < 2 {
            return None;
        }
        let palette = Theme::of(cx).palette(Surface::Chrome);
        Some(host_picker(
            &hosts,
            &current,
            palette,
            &Self::row_handler(cx),
        ))
    }

    fn current_host(
        &self,
        hosts: &[Host],
    ) -> Option<Host> {
        let active = self.data.active_project()?;
        let project = self
            .data
            .projects()
            .into_iter()
            .find(|project| project.id == active)?;
        hosts.iter().find(|host| host.id == project.host).cloned()
    }
}

fn traffic_light_reserve(side: DockSide) -> f32 {
    if side == DockSide::Right {
        return 0.0;
    }
    TRAFFIC_LIGHT_RESERVE
}

#[cfg(test)]
mod tests {
    use super::traffic_light_reserve;
    use crate::dock_side::DockSide;
    use crate::metrics::TRAFFIC_LIGHT_RESERVE;
    use crate::sidebar::Sidebar;
    use crate::test_support::prelude::*;
    use crate::visibility::Visibility;

    #[test]
    fn only_a_left_dock_reserves_room_for_the_traffic_lights() {
        let cases = [
            (DockSide::Left, TRAFFIC_LIGHT_RESERVE),
            (DockSide::Right, 0.0),
        ];
        for (side, reserve) in cases {
            assert_eq!(traffic_light_reserve(side), reserve, "{side:?}");
        }
    }

    #[gpui::test]
    fn the_toggle_closes_and_reopens_the_sidebar(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, Sidebar::toggle);
        assert!(sidebar.read_with(cx, |this, _| this.panel.visibility() == Visibility::Closed));
        sidebar.update(cx, Sidebar::toggle);
        assert!(sidebar.read_with(cx, |this, _| this.panel.visibility() == Visibility::Open));
    }

    #[gpui::test]
    fn the_current_host_is_the_one_of_the_active_space(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::rich());
        let name = sidebar.read_with(cx, |this, _| {
            let hosts = this.data.hosts();
            this.current_host(&hosts).map(|host| host.name)
        });
        assert!(name.is_some());
    }

    #[gpui::test]
    fn a_single_host_shows_no_picker(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        let picker = sidebar.update(cx, |this, cx| this.host_choice(cx));
        assert!(picker.is_none());
    }
}
