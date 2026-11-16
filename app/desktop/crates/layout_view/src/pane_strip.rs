use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, RenderOnce, Styled, Window, div, px,
};
use ui::{Surface, Theme};

use crate::strip_tab::StripTab;
use crate::tab_actions::TabDrop;

pub(crate) const STRIP_HEIGHT: f32 = 42.0;
const PAD_X: f32 = 8.0;
const PAD_Y: f32 = 4.0;
const GAP: f32 = 6.0;

#[derive(IntoElement)]
pub(crate) struct PaneStrip {
    pub(crate) tabs: Vec<StripTab>,
    pub(crate) at_end_drop: TabDrop,
}

impl RenderOnce for PaneStrip {
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let at_end = self.at_end_drop;
        div()
            .id("pane-strip")
            .flex()
            .items_center()
            .gap(px(GAP))
            .px(px(PAD_X))
            .py(px(PAD_Y))
            .w_full()
            .h(px(STRIP_HEIGHT))
            .overflow_hidden()
            .bg(palette.pane_chrome_bg())
            .on_drop::<model::DragSubject>(move |subject, window, cx| at_end(subject, window, cx))
            .children(self.tabs)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::strip_tab::HEIGHT as TAB_HEIGHT;

    #[test]
    fn the_strip_row_holds_a_tab_with_its_padding() {
        assert!(
            std::hint::black_box(STRIP_HEIGHT) >= TAB_HEIGHT + 2.0 * PAD_Y,
            "row too short"
        );
    }
}
