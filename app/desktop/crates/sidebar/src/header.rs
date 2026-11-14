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
use crate::metrics::HEADER_HEIGHT;
use crate::row_action::RowAction;
use crate::sidebar::Sidebar;
use crate::visibility::Visibility;

impl Sidebar {
    pub(crate) fn header(&mut self, cx: &mut Context<'_, Self>) -> AnyElement {
        let side = self.panel.side();
        let picker = self.host_choice(cx);
        div()
            .flex()
            .flex_none()
            .items_center()
            .gap(px(4.0))
            .h(px(HEADER_HEIGHT))
            .w_full()
            .overflow_hidden()
            .window_control_area(WindowControlArea::Drag)
            .when(side == DockSide::Left, |d| d.pl(px(12.0)).pr(px(8.0)))
            .when(side == DockSide::Right, |d| {
                d.flex_row_reverse().pr(px(12.0)).pl(px(8.0))
            })
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

    fn toggle_button(&self, cx: &mut Context<'_, Self>) -> IconButton {
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

    fn host_choice(&self, cx: &mut Context<'_, Self>) -> Option<AnyElement> {
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

    fn current_host(&self, hosts: &[Host]) -> Option<Host> {
        let active = self.data.active_project()?;
        let project = self
            .data
            .projects()
            .into_iter()
            .find(|project| project.id == active)?;
        hosts.iter().find(|host| host.id == project.host).cloned()
    }
}

#[cfg(test)]
#[path = "header_tests.rs"]
mod tests;
