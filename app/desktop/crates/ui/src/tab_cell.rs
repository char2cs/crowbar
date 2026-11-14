//! One tab of a [`crate::Tabs`] list.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, ElementId, FontWeight, Hsla, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, Stateful, StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::focus_ring::focus_ring;
use crate::font::UI_FAMILY;
use crate::hover;
use crate::icon::Icon;
use crate::icon_weight::IconWeight;
use crate::index_handler::IndexHandler;
use crate::opaque_debug::opaque_debug;
use crate::palette::Palette;
use crate::pose::Pose;
use crate::radius;
use crate::tab_item::TabItem;
use crate::tabs_style::TabsStyle;
use crate::text_size;

const DISABLED_OPACITY: f32 = 0.64;
const MUTED_TEXT: f32 = 0.72;
const ICON_WITH_LABEL: f32 = 16.0;
const ICON_ALONE: f32 = 14.0;

#[derive(IntoElement)]
pub(crate) struct TabCell {
    id: ElementId,
    item: TabItem,
    index: usize,
    selected: Pose,
    look: (TabsStyle, &'static Palette),
    on_select: Option<IndexHandler>,
}

impl TabCell {
    pub(crate) fn new(id: ElementId, item: TabItem, look: (TabsStyle, &'static Palette)) -> Self {
        Self {
            id,
            item,
            index: 0,
            selected: Pose::Off,
            look,
            on_select: None,
        }
    }

    pub(crate) fn at(mut self, index: usize, selected: Pose) -> Self {
        self.index = index;
        self.selected = selected;
        self
    }

    pub(crate) fn on_select(mut self, handler: Option<IndexHandler>) -> Self {
        self.on_select = handler;
        self
    }
}

impl TabCell {
    fn frame(&self, color: Hsla) -> Stateful<Div> {
        let (style, palette) = self.look;
        let frame = div()
            .id(self.id.clone())
            .relative()
            .flex()
            .flex_1()
            .items_center()
            .justify_center()
            .gap(px(6.0))
            .h(px(style.tab_height()))
            .px(px(8.0))
            .rounded(radius::MD)
            .text_color(color)
            .text_size(text_size::SM)
            .font_family(UI_FAMILY)
            .font_weight(FontWeight::MEDIUM)
            .whitespace_nowrap();
        frame
            .focus(move |s| s.shadow(focus_ring(palette)))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
    }

    fn icon(&self, color: Hsla) -> Option<Icon> {
        let selected = self.selected == Pose::On;
        let weight = if selected {
            self.item.selected_weight
        } else {
            IconWeight::Regular
        };
        let size = if self.item.label.is_some() {
            ICON_WITH_LABEL
        } else {
            ICON_ALONE
        };
        self.item
            .icon
            .map(|icon| Icon::new(icon).weight(weight).color(color).size(px(size)))
    }
}

impl RenderOnce for TabCell {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let (style, palette) = self.look;
        let selected = self.selected == Pose::On;
        let hover_state = hover::track(&self.id, Pose::Off, window, cx);
        let rest = palette.muted_foreground().opacity(MUTED_TEXT);
        let color = if selected {
            style.active_text(palette)
        } else {
            hover::color(&hover_state, (rest, palette.foreground()), window, cx)
        };
        let (index, disabled) = (self.index, self.item.disabled);
        let handler = self.on_select.clone().filter(|_| !selected && !disabled);
        self.frame(color)
            .when(!disabled, |d| {
                d.cursor_pointer()
                    .tab_stop(true)
                    .on_hover(hover::listener(&hover_state))
            })
            .when(disabled, |d| d.opacity(DISABLED_OPACITY))
            .children(self.icon(color))
            .children(self.item.label)
            .when_some(handler, move |d, handler| {
                d.on_click(move |_, window, cx| handler(index, window, cx))
            })
    }
}

opaque_debug!(TabCell);

#[cfg(test)]
#[path = "tab_cell_tests.rs"]
mod tests;
