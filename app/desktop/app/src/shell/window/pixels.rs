use gpui::{Bounds, Pixels};

pub(crate) fn to_bounds(bounds: Bounds<Pixels>) -> webview::Bounds {
    webview::Bounds {
        x: f64::from(f32::from(bounds.origin.x)),
        y: f64::from(f32::from(bounds.origin.y)),
        width: f64::from(f32::from(bounds.size.width)),
        height: f64::from(f32::from(bounds.size.height)),
    }
}

#[cfg(test)]
mod tests {
    use gpui::{point, px, size};

    use super::*;

    #[test]
    fn gpui_bounds_become_logical_web_view_bounds() {
        let bounds = Bounds {
            origin: point(px(294.0), px(42.0)),
            size: size(px(800.0), px(600.0)),
        };
        assert_eq!(
            to_bounds(bounds),
            webview::Bounds {
                x: 294.0,
                y: 42.0,
                width: 800.0,
                height: 600.0
            },
            "converted"
        );
    }
}
