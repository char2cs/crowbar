//! The host switcher in the header, shown when two or more hosts exist.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Div, InteractiveElement, IntoElement, ParentElement, Pixels, Styled, div, px,
};
use ui::{DropdownMenu, Icon, IconName, IconWeight, MenuAction, MenuItem, Palette, radius};

use crate::command::Command;
use crate::host::Host;
use crate::host_kind::HostKind;
use crate::host_tone::HostTone;
use crate::intent::Intent;
use crate::row_action::{RowAction, RowHandler};
use crate::scale::{ICON_LARGE, SPACE_MD, SPACE_SM, TEXT_SMALL};

const DOT_OFFSET: f32 = -1.0;
const TRIGGER_HEIGHT: Pixels = px(24.0);
const NAME_MAX_WIDTH: Pixels = px(96.0);
const MENU_MIN_WIDTH: Pixels = px(224.0);

const DOT: f32 = 5.0;
const CUTOUT: f32 = 8.0;
const TRIGGER_GROUP: &str = "host-trigger";
const UNSETTLED_ALPHA: f32 = 0.65;

fn dot(
    host: &Host,
    palette: &Palette,
) -> Div {
    let base = div()
        .absolute()
        .right(px(DOT_OFFSET))
        .bottom(px(DOT_OFFSET))
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

pub(crate) fn host_icon(
    host: &Host,
    palette: &Palette,
) -> Div {
    let name = match host.kind {
        HostKind::Local => IconName::Desktop,
        HostKind::Remote => IconName::Cloud,
    };
    div()
        .relative()
        .flex_none()
        .size(ICON_LARGE)
        .child(Icon::new(name).weight(IconWeight::Regular).size(ICON_LARGE))
        .child(cutout(palette))
        .child(dot(host, palette))
}

fn cutout_offset() -> f32 {
    DOT_OFFSET - (CUTOUT - DOT) / 2.0
}

fn cutout(palette: &Palette) -> Div {
    let hovered = palette.chrome_bg().blend(palette.sidebar_element_hover());
    div()
        .absolute()
        .right(px(cutout_offset()))
        .bottom(px(cutout_offset()))
        .size(px(CUTOUT))
        .rounded(radius::FULL)
        .bg(palette.chrome_bg())
        .group_hover(TRIGGER_GROUP, move |style| style.bg(hovered))
}

fn item(
    host: &Host,
    current: &Host,
    handler: &RowHandler,
) -> MenuItem {
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
        .group(TRIGGER_GROUP)
        .flex()
        .items_center()
        .gap(SPACE_SM)
        .h(TRIGGER_HEIGHT)
        .px(SPACE_MD)
        .rounded(radius::FULL)
        .text_size(TEXT_SMALL)
        .text_color(palette.muted_foreground())
        .hover(hovered)
        .child(host_icon(current, palette))
        .child(
            div()
                .max_w(NAME_MAX_WIDTH)
                .truncate()
                .child(current.name.clone()),
        );
    DropdownMenu::new("host-picker", trigger, items)
        .min_width(MENU_MIN_WIDTH)
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    use crate::host::Host;
    use crate::host_id::HostId;
    use crate::host_kind::HostKind;
    use crate::host_state::HostState;
    use crate::test_support::measure;

    fn host(kind: HostKind) -> Host {
        Host {
            id: HostId::from("h"),
            name: "h".into(),
            kind,
            state: HostState::Up,
            endpoint: None,
        }
    }

    #[test]
    fn the_cutout_is_centred_on_the_dot() {
        let dot_centre = DOT_OFFSET + DOT / 2.0;
        let cutout_centre = cutout_offset() + CUTOUT / 2.0;
        assert!((dot_centre - cutout_centre).abs() < f32::EPSILON);
    }

    #[gpui::test]
    fn the_host_icon_is_sixteen_pixels_tall(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, cx| {
            let palette = ui::Theme::of(cx).palette(ui::Surface::Chrome);
            host_icon(&host(HostKind::Remote), palette)
        });
        assert_eq!(f32::from(bounds.size.height), 16.0);
    }
}
