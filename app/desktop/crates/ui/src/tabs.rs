//! Equal-width tab list with a sliding selection pill.

use std::time::Instant;

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, FontWeight, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px, relative,
};

use crate::cubic_bezier::CubicBezier;
use crate::focus_ring::focus_ring;
use crate::font::UI_FAMILY;
use crate::hover;
use crate::icon::Icon;
use crate::index_handler::IndexHandler;
use crate::motion::Motion;
use crate::palette::Palette;
use crate::radius;
use crate::shadow;
use crate::slide_state::SlideState;
use crate::surface::Surface;
use crate::tab_item::TabItem;
use crate::tabs_style::TabsStyle;
use crate::text_size;
use crate::theme::Theme;
use crate::timing;

const GAP: f32 = 2.0;
const DISABLED_OPACITY: f32 = 0.64;
const MUTED_TEXT: f32 = 0.72;

/// A list of equal-width tabs; the owner keeps the selected index.
#[derive(IntoElement)]
pub struct Tabs {
    id: ElementId,
    items: Vec<TabItem>,
    selected: usize,
    style: TabsStyle,
    surface: Surface,
    on_select: Option<IndexHandler>,
}

impl Tabs {
    /// A pill-style list showing `items` with `selected` active.
    pub fn new(id: impl Into<ElementId>, items: Vec<TabItem>, selected: usize) -> Self {
        Self { id: id.into(), items, selected, style: TabsStyle::default(), surface: Surface::default(), on_select: None }
    }

    /// Chooses the visual recipe.
    #[must_use]
    pub fn style(mut self, style: TabsStyle) -> Self {
        self.style = style;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(mut self, surface: Surface) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the index of the clicked tab.
    #[must_use]
    pub fn on_select(mut self, handler: impl Fn(usize, &mut Window, &mut App) + 'static) -> Self {
        self.on_select = Some(std::rc::Rc::new(handler));
        self
    }
}

fn pill(style: TabsStyle, palette: &Palette) -> gpui::Div {
    match style {
        TabsStyle::Pill => div()
            .rounded(radius::LG)
            .border_1()
            .border_color(palette.control_surface())
            .bg(palette.control_surface())
            .shadow(shadow::xs(0.1)),
        TabsStyle::Segmented => div().rounded(radius::MD).bg(palette.background()).shadow(shadow::sm(0.05)),
    }
}

fn tab_height(style: TabsStyle) -> f32 {
    match style {
        TabsStyle::Pill => 32.0,
        TabsStyle::Segmented => 30.0,
    }
}

impl RenderOnce for Tabs {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = *Theme::of(cx);
        let palette = theme.palette(self.surface);
        let motion = Motion::of(cx);
        let count = self.items.len().max(1);
        let slide = window.use_keyed_state((self.id.clone(), "slide"), cx, |_, _| SlideState::starting(self.selected));
        slide.update(cx, |s, cx| s.go_to(self.selected, timing::SLOW, CubicBezier::DEFAULT, motion, cx));
        let (position, running) = slide.read(cx).at(timing::SLOW, CubicBezier::DEFAULT, motion, Instant::now());
        if running {
            window.request_animation_frame();
        }
        let height = tab_height(self.style);
        let indicator = div()
            .absolute()
            .top_0()
            .bottom_0()
            .right(px(-GAP))
            .left_0()
            .child(
                div()
                    .absolute()
                    .top_0()
                    .h_full()
                    .w(relative(1.0 / count as f32))
                    .left(relative(position / count as f32))
                    .child(pill(self.style, palette).h_full().mr(px(GAP))),
            );
        let tabs = self.items.into_iter().enumerate().map(|(index, item)| {
            let handler = self.on_select.clone();
            let selected = index == self.selected;
            let id = (self.id.clone(), index);
            tab(id, item, selected, (self.style, palette), window, cx)
                .when_some(handler.filter(|_| !selected), move |d, handler| {
                    d.on_click(move |_, window, cx| handler(index, window, cx))
                })
        });
        div()
            .id(self.id)
            .relative()
            .flex()
            .w_full()
            .gap(px(GAP))
            .p(px(GAP))
            .rounded(radius::LG)
            .bg(palette.muted())
            .child(div().absolute().top(px(GAP)).bottom(px(GAP)).left(px(GAP)).right(px(GAP)).child(indicator))
            .children(tabs)
            .min_h(px(height + 2.0 * GAP))
    }
}

fn tab(
    id: (ElementId, usize),
    item: TabItem,
    selected: bool,
    (style, palette): (TabsStyle, &'static Palette),
    window: &mut Window,
    cx: &mut App,
) -> gpui::Stateful<gpui::Div> {
    let hover_state = hover::track(&id.0, "tab-hover", false, window, cx);
    let (active, rest) = match style {
        TabsStyle::Pill => (palette.control_foreground(), palette.muted_foreground().opacity(MUTED_TEXT)),
        TabsStyle::Segmented => (palette.foreground(), palette.muted_foreground().opacity(MUTED_TEXT)),
    };
    let color = if selected { active } else { hover::color(&hover_state, rest, palette.foreground(), window, cx) };
    let weight = if selected { item.selected_weight } else { crate::icon_weight::IconWeight::Regular };
    div()
        .id(id)
        .relative()
        .flex()
        .flex_1()
        .items_center()
        .justify_center()
        .gap(px(6.0))
        .h(px(tab_height(style)))
        .px(px(8.0))
        .rounded(radius::MD)
        .text_color(color)
        .text_size(text_size::SM)
        .font_family(UI_FAMILY)
        .font_weight(FontWeight::MEDIUM)
        .whitespace_nowrap()
        .when(!item.disabled, |d| d.cursor_pointer().tab_stop(true).on_hover(hover::listener(&hover_state)))
        .when(item.disabled, |d| d.opacity(DISABLED_OPACITY))
        .focus(move |s| s.shadow(focus_ring(palette)))
        .on_mouse_down(MouseButton::Left, |_, window, _| window.prevent_default())
        .children(item.icon.map(|icon| Icon::new(icon).weight(weight).color(color).size(px(if item.label.is_some() { 16.0 } else { 14.0 }))))
        .children(item.label)
}

#[cfg(test)]
#[path = "tabs_tests.rs"]
mod tests;
