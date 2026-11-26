//! The strip reserved above a pane's content.

use gpui::{AnyView, App, Window};
use protocol::PaneId;

/// The strip reserved above a pane's content.
pub trait PaneChrome {
    /// Height in logical pixels of the strip reserved at the top of the pane's content for
    /// [`PaneChrome::chrome`]; the pane's hole starts below it.
    fn chrome_height(
        &self,
        _pane: &PaneId,
    ) -> f32 {
        0.0
    }

    /// The view drawn in the chrome strip, such as a browser address bar. Called only for panes
    /// whose [`PaneChrome::chrome_height`] is above zero.
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
