use gpui::{point, size};

use super::*;

#[test]
fn gpui_bounds_become_logical_web_view_bounds() {
    let bounds = Bounds { origin: point(px(220.0), px(36.0)), size: size(px(800.0), px(600.0)) };
    assert_eq!(
        content_bounds(bounds),
        webview::Bounds { x: 220.0, y: 36.0, width: 800.0, height: 600.0 },
        "converted"
    );
}
