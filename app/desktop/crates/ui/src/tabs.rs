//! Equal-width tab list with a sliding selection pill.

use std::time::Instant;

use gpui::{
    App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    Styled, Window, div, px,
};

use crate::cubic_bezier::CubicBezier;
use crate::index_handler::IndexHandler;
use crate::motion::Motion;
use crate::opaque_debug::opaque_debug;
use crate::pose::Pose;
use crate::radius;
use crate::slide_state::SlideState;
use crate::surface::Surface;
use crate::tab_cell::TabCell;
use crate::tab_indicator::{GAP, tab_indicator};
use crate::tab_item::TabItem;
use crate::tabs_style::TabsStyle;
use crate::theme::Theme;
use crate::timing;
use crate::tween::Tween;

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
        Self {
            id: id.into(),
            items,
            selected,
            style: TabsStyle::default(),
            surface: Surface::default(),
            on_select: None,
        }
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

fn slide_position(id: &ElementId, selected: u16, window: &mut Window, cx: &mut App) -> f32 {
    let tween = Tween::new(timing::SLOW, CubicBezier::DEFAULT);
    let slide = window.use_keyed_state((id.clone(), "slide"), cx, |_, _| {
        SlideState::starting(selected)
    });
    slide.update(cx, |state, cx| state.go_to(selected, tween, cx));
    let (position, running) = slide.read(cx).at(tween, Motion::of(cx), Instant::now());
    if running {
        window.request_animation_frame();
    }
    position
}

impl RenderOnce for Tabs {
    fn render(self, window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let selected = u16::try_from(self.selected).unwrap_or(u16::MAX);
        let count = u16::try_from(self.items.len()).unwrap_or(u16::MAX);
        let position = slide_position(&self.id, selected, window, cx);
        let look = (self.style, palette);
        let id = self.id.clone();
        let cells = self.items.into_iter().enumerate().map(|(index, item)| {
            let id = ElementId::from((id.clone(), SharedString::from(format!("tab-{index}"))));
            let pose = Pose::from(index == self.selected);
            TabCell::new(id, item, look)
                .at(index, pose)
                .on_select(self.on_select.clone())
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
            .min_h(px(self.style.tab_height() + 2.0 * GAP))
            .child(
                div()
                    .absolute()
                    .inset(px(GAP))
                    .child(tab_indicator(self.style, palette, count, position)),
            )
            .children(cells)
    }
}

opaque_debug!(Tabs);

#[cfg(test)]
#[path = "tabs_tests.rs"]
mod tests;
