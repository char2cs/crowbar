//! One tab of a [`crate::Tabs`] list.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, Div, ElementId, Entity, FontWeight, Hsla, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, Stateful, StatefulInteractiveElement, Styled, Window, div, px,
};

use crate::focus_ring::focus_ring;
use crate::hover;
use crate::icon::Icon;
use crate::icon_weight::IconWeight;
use crate::index_handler::IndexHandler;
use crate::opaque_debug::opaque_debug;
use crate::palette::Palette;
use crate::pose::Pose;
use crate::radius;
use crate::spacing;
use crate::tab_item::TabItem;
use crate::tabs_style::TabsStyle;
use crate::text_size;
use crate::theme::Theme;
use crate::toggle_state::ToggleState;

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
    pub(crate) fn new(
        id: ElementId,
        item: TabItem,
        look: (TabsStyle, &'static Palette),
    ) -> Self {
        Self {
            id,
            item,
            index: 0,
            selected: Pose::Off,
            look,
            on_select: None,
        }
    }

    pub(crate) fn at(
        mut self,
        index: usize,
        selected: Pose,
    ) -> Self {
        self.index = index;
        self.selected = selected;
        self
    }

    pub(crate) fn on_select(
        mut self,
        handler: Option<IndexHandler>,
    ) -> Self {
        self.on_select = handler;
        self
    }
}

impl TabCell {
    fn frame(
        &self,
        color: Hsla,
    ) -> Stateful<Div> {
        let (style, palette) = self.look;
        let frame = div()
            .id(self.id.clone())
            .relative()
            .flex()
            .flex_1()
            .items_center()
            .justify_center()
            .gap(spacing::STEP_1_5)
            .h(px(style.tab_height()))
            .px(spacing::STEP_2)
            .rounded(radius::MD)
            .text_color(color)
            .text_size(text_size::SM)
            .font_weight(FontWeight::MEDIUM)
            .whitespace_nowrap();
        frame
            .focus(move |s| s.shadow(focus_ring(palette)))
            .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
    }

    fn label_color(
        &self,
        hover_state: &Entity<ToggleState>,
        window: &mut Window,
        cx: &App,
    ) -> Hsla {
        let (style, palette) = self.look;
        if self.selected == Pose::On {
            return style.active_text(palette);
        }
        let rest = palette.muted_foreground().opacity(MUTED_TEXT);
        hover::color(hover_state, rest..palette.foreground(), window, cx)
    }

    fn icon_weight(&self) -> IconWeight {
        if self.selected == Pose::On {
            return self.item.selected_weight;
        }
        IconWeight::Regular
    }

    fn icon_size(&self) -> f32 {
        if self.item.label.is_some() {
            return ICON_WITH_LABEL;
        }
        ICON_ALONE
    }

    fn icon(
        &self,
        color: Hsla,
    ) -> Option<Icon> {
        let weight = self.icon_weight();
        let size = self.icon_size();
        self.item
            .icon
            .map(|icon| Icon::new(icon).weight(weight).color(color).size(px(size)))
    }
}

impl RenderOnce for TabCell {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let selected = self.selected == Pose::On;
        let hover_state = hover::track(&self.id, Pose::Off, window, cx);
        let color = self.label_color(&hover_state, window, cx);
        let (index, disabled) = (self.index, self.item.disabled);
        let handler = self.on_select.clone().filter(|_| !selected && !disabled);
        self.frame(color)
            .font_family(Theme::of(cx).ui_family())
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
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Modifiers, TestAppContext, point, px};

    use super::*;
    use crate::icon_name::IconName;
    use crate::surface::Surface;
    use crate::test_support::{measure, open};
    use crate::theme::Theme;

    fn look() -> (TabsStyle, &'static Palette) {
        (TabsStyle::Pill, Theme::default().palette(Surface::Window))
    }

    fn cell() -> TabCell {
        TabCell::new(
            ElementId::from("tab"),
            TabItem::icon(IconName::Gear),
            look(),
        )
    }

    #[gpui::test]
    fn height_follows_the_style(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| cell());
        assert!((f32::from(bounds.size.height) - 32.0).abs() < f32::EPSILON);
    }

    #[gpui::test]
    fn only_an_enabled_unselected_tab_reports_clicks(cx: &mut TestAppContext) {
        let clicked = Rc::new(Cell::new(0));
        let sink = Rc::clone(&clicked);
        let handler: IndexHandler = Rc::new(move |_, _, _| sink.set(sink.get() + 1));
        let cx = open(cx, move |_, _| {
            cell().at(3, Pose::Off).on_select(Some(Rc::clone(&handler)))
        });
        cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
        assert_eq!(clicked.get(), 1);
    }

    #[gpui::test]
    fn selected_tab_ignores_clicks(cx: &mut TestAppContext) {
        let clicked = Rc::new(Cell::new(0));
        let sink = Rc::clone(&clicked);
        let handler: IndexHandler = Rc::new(move |_, _, _| sink.set(sink.get() + 1));
        let cx = open(cx, move |_, _| {
            cell().at(0, Pose::On).on_select(Some(Rc::clone(&handler)))
        });
        cx.simulate_click(point(px(10.0), px(10.0)), Modifiers::none());
        assert_eq!(clicked.get(), 0);
    }
}
