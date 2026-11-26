//! Where the panes' web views sit in the window, and what the window's overlays cover of them.

use gpui::{App, Entity, Window};

pub(crate) struct PaneArea {
    pub(crate) placement: Option<(webview::ParentWindow, webview::Bounds)>,
    pub(crate) cover: Entity<ui::Cover>,
}

impl PaneArea {
    pub(crate) fn unplaced(
        window: &Window,
        cx: &mut App,
    ) -> Self {
        Self {
            placement: None,
            cover: ui::Cover::of(window, cx),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_panes_follow_the_cover_the_window_s_overlays_open_in(cx: &mut TestAppContext) {
        let window = cx.add_empty_window();
        let (area, cover) =
            window.update(|window, cx| (PaneArea::unplaced(window, cx), ui::Cover::of(window, cx)));
        assert!(
            area.placement.is_none(),
            "placed once the window reports it"
        );
        assert_eq!(area.cover, cover);
    }
}
