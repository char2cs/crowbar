//! The host switcher in the header, shown when two or more hosts exist.

use std::rc::Rc;

use gpui::{AnyElement, App, Div, InteractiveElement, IntoElement, ParentElement, Styled, div, px};
use ui::{DropdownMenu, Icon, IconName, IconWeight, MenuAction, MenuItem, Palette, radius};

use crate::command::Command;
use crate::host::Host;
use crate::host_kind::HostKind;
use crate::host_tone::HostTone;
use crate::intent::Intent;
use crate::row_action::{RowAction, RowHandler};

const DOT: f32 = 5.0;
const UNSETTLED_ALPHA: f32 = 0.65;

fn dot(host: &Host, palette: &Palette) -> Div {
    let base = div()
        .absolute()
        .right(px(-1.0))
        .bottom(px(-1.0))
        .size(px(DOT))
        .rounded(radius::FULL)
        .border_1();
    match host.state.tone() {
        HostTone::Online => base.border_color(palette.success()).bg(palette.success()),
        HostTone::Offline => base
            .border_color(palette.destructive())
            .bg(palette.destructive()),
        HostTone::NeedsAttention => base.border_color(palette.warning()).bg(palette.warning()),
        HostTone::Unsettled => {
            base.border_color(palette.muted_foreground().opacity(UNSETTLED_ALPHA))
        },
    }
}

pub(crate) fn host_icon(host: &Host, palette: &Palette) -> Div {
    let name = match host.kind {
        HostKind::Local => IconName::Desktop,
        HostKind::Remote => IconName::Cloud,
    };
    div()
        .relative()
        .flex_none()
        .size(px(16.0))
        .child(Icon::new(name).weight(IconWeight::Regular).size(px(16.0)))
        .child(dot(host, palette))
}

fn item(host: &Host, current: &Host, handler: &RowHandler) -> MenuItem {
    let (id, handler) = (host.id.clone(), Rc::clone(handler));
    let action = MenuAction::new(host.name.clone(), move |window, cx: &mut App| {
        let command = Command::SelectHost(id.clone());
        handler(RowAction::Intent(Intent::Send(command)), window, cx);
    });
    if host.id == current.id {
        return MenuItem::Action(action.checked());
    }
    MenuItem::Action(action)
}

pub(crate) fn host_picker(
    hosts: &[Host],
    current: &Host,
    palette: &'static Palette,
    handler: &RowHandler,
) -> AnyElement {
    let items: Vec<MenuItem> = hosts
        .iter()
        .map(|host| item(host, current, handler))
        .collect();
    let hovered = move |style: gpui::StyleRefinement| {
        style
            .bg(palette.sidebar_element_hover())
            .text_color(palette.foreground())
    };
    let trigger = div()
        .flex()
        .items_center()
        .gap(px(6.0))
        .h(px(24.0))
        .px(px(8.0))
        .rounded(radius::FULL)
        .text_size(px(12.0))
        .text_color(palette.muted_foreground())
        .hover(hovered)
        .child(host_icon(current, palette))
        .child(div().max_w(px(96.0)).truncate().child(current.name.clone()));
    DropdownMenu::new("host-picker", trigger, items)
        .min_width(px(224.0))
        .into_any_element()
}

#[cfg(test)]
#[path = "host_picker_tests.rs"]
mod tests;
