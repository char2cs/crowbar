use gpui::{AnyElement, App, IntoElement as _, Window};
use protocol::PaneId;

pub trait PaneContent {
    fn content(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement;

    fn is_native(
        &self,
        _pane: &PaneId,
    ) -> bool {
        false
    }

    fn empty_editor(
        &self,
        _chat: &PaneId,
        _window: &mut Window,
        _cx: &mut App,
    ) -> AnyElement {
        gpui::Empty.into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use gpui::div;

    use super::*;
    use crate::test_support::pane_id;

    struct Hosted;

    impl PaneContent for Hosted {
        fn content(
            &self,
            _pane: &PaneId,
            _window: &mut Window,
            _cx: &mut App,
        ) -> AnyElement {
            div().into_any_element()
        }
    }

    #[test]
    fn a_pane_is_hosted_by_a_web_view_unless_it_says_otherwise() {
        assert!(!Hosted.is_native(&pane_id("a")));
    }
}
