//! Equal-width tab list with a sliding selection pill.

mod cell;
mod indicator;
mod slide_state;
mod tab_item;
mod tabs_style;

pub use tab_item::TabItem;
pub use tabs_style::TabsStyle;

use gpui::{
    App, ElementId, InteractiveElement, IntoElement, ParentElement, RenderOnce, SharedString,
    Styled, Window, div, px,
};

use crate::control::tabs::cell::Cell;
use crate::control::tabs::indicator::{GAP, tab_indicator};
use crate::control::tabs::slide_state::SlideState;
use crate::foundation::CubicBezier;
use crate::foundation::IndexHandler;
use crate::foundation::Motion;
use crate::foundation::Pose;
use crate::foundation::Surface;
use crate::foundation::Theme;
use crate::foundation::Tween;
use crate::foundation::motion::adapters::clock as system_clock;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::radius;
use crate::foundation::tokens::timing;

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
    pub fn new(
        id: impl Into<ElementId>,
        items: Vec<TabItem>,
        selected: usize,
    ) -> Self {
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
    pub fn style(
        mut self,
        style: TabsStyle,
    ) -> Self {
        self.style = style;
        self
    }

    /// Paints with the palette of `surface`.
    #[must_use]
    pub fn on(
        mut self,
        surface: Surface,
    ) -> Self {
        self.surface = surface;
        self
    }

    /// Registers the handler receiving the index of the clicked tab.
    #[must_use]
    pub fn on_select(
        mut self,
        handler: impl Fn(usize, &mut Window, &mut App) + 'static,
    ) -> Self {
        self.on_select = Some(std::rc::Rc::new(handler));
        self
    }
}

fn slide_position(
    id: &ElementId,
    selected: u16,
    window: &mut Window,
    cx: &mut App,
) -> f32 {
    let tween = Tween::new(timing::SLOW, CubicBezier::DEFAULT);
    let slide = window.use_keyed_state((id.clone(), "slide"), cx, |_, _| {
        SlideState::starting(selected)
    });
    slide.update(cx, |state, cx| state.go_to(selected, tween, cx));
    let (position, running) = slide
        .read(cx)
        .at(tween, Motion::of(cx), system_clock::now());
    if running {
        window.request_animation_frame();
    }
    position
}

impl RenderOnce for Tabs {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(self.surface);
        let selected = u16::try_from(self.selected).unwrap_or(u16::MAX);
        let count = u16::try_from(self.items.len()).unwrap_or(u16::MAX);
        let position = slide_position(&self.id, selected, window, cx);
        let look = (self.style, palette);
        let id = self.id.clone();
        let cells = self.items.into_iter().enumerate().map(|(index, item)| {
            let id = ElementId::from((id.clone(), SharedString::from(format!("tab-{index}"))));
            let pose = Pose::from(index == self.selected);
            Cell::new(id, item, look)
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
            .bg(self.style.track(palette))
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
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{ParentElement, Styled, TestAppContext, div, point, px};

    use super::*;
    use crate::foundation::IconName;
    use crate::test_support::probe::{measure, open};

    fn items() -> Vec<TabItem> {
        vec![
            TabItem::icon(IconName::ChatsCircle),
            TabItem::icon(IconName::FolderOpen),
            TabItem::icon(IconName::GitBranch),
        ]
    }

    #[gpui::test]
    fn list_is_thirty_six_pixels_high_in_pill_style_and_thirty_four_segmented(
        cx: &mut TestAppContext
    ) {
        let pill = measure(cx, |_, _| {
            div().w(px(300.0)).child(Tabs::new("t", items(), 0))
        });
        assert_eq!(f32::from(pill.size.height), 36.0);
        let segmented = measure(cx, |_, _| {
            div()
                .w(px(300.0))
                .child(Tabs::new("t", items(), 0).style(TabsStyle::Segmented))
        });
        assert_eq!(f32::from(segmented.size.height), 34.0);
    }

    #[gpui::test]
    fn clicking_an_unselected_tab_reports_its_index(cx: &mut TestAppContext) {
        let seen = Rc::new(Cell::new(None));
        let sink = Rc::clone(&seen);
        let cx = open(cx, move |_, _| {
            let sink = Rc::clone(&sink);
            div().w(px(300.0)).child(
                Tabs::new("t", items(), 0).on_select(move |index, _, _| sink.set(Some(index))),
            )
        });
        cx.simulate_click(point(px(150.0), px(18.0)), gpui::Modifiers::none());
        assert_eq!(seen.get(), Some(1));
        seen.set(None);
        cx.simulate_click(point(px(20.0), px(18.0)), gpui::Modifiers::none());
        assert_eq!(seen.get(), None, "the selected tab does not re-select");
    }
}
