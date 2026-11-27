use gpui::{AnyView, App, Window};
use protocol::PaneId;

pub trait PaneChrome {
    fn chrome_height(
        &self,
        _pane: &PaneId,
    ) -> f32 {
        0.0
    }

    fn chrome(
        &self,
        _pane: &PaneId,
        _window: &mut Window,
        _cx: &mut App,
    ) -> Option<AnyView> {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::pane_id;

    struct Bare;

    impl PaneChrome for Bare {}

    #[test]
    fn a_pane_reserves_no_chrome_strip_unless_it_says_otherwise() {
        assert_eq!(Bare.chrome_height(&pane_id("a")), 0.0);
    }
}
