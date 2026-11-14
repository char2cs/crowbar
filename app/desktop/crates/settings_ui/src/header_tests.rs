use gpui::TestAppContext;

use super::*;
use crate::test_support::open;

#[gpui::test]
fn the_dialog_never_overflows_the_window(cx: &mut TestAppContext) {
    let (_, cx) = open(cx, "appearance");
    cx.update(|window, _| {
        let viewport = window.viewport_size();
        let size = extent(window).size(viewport);
        assert!(size.width <= viewport.width && size.height <= viewport.height);
    });
}
