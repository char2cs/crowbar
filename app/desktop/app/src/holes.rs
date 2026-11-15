use layout_view::{ChromeContext, pane_holes};
use model::{Rect, Tab};
use pane_host::VisiblePane;
use webview::Bounds;

/// The window rectangles the web views of `tab` fill: the pane holes of the layout view, which
/// occupies `area` of the window.
pub(crate) fn visible_panes(
    tab: Option<&Tab>,
    area: Bounds,
    chrome: ChromeContext,
) -> Vec<VisiblePane> {
    let Some(tab) = tab else {
        return Vec::new();
    };
    #[expect(
        clippy::cast_possible_truncation,
        reason = "window sizes in logical pixels are far below f32 precision limits"
    )]
    let local = Rect {
        x: 0.0,
        y: 0.0,
        width: area.width as f32,
        height: area.height as f32,
    };
    pane_holes(tab, local, chrome)
        .into_iter()
        .map(|hole| VisiblePane {
            pane: hole.pane,
            bounds: Bounds {
                x: area.x + f64::from(hole.rect.x),
                y: area.y + f64::from(hole.rect.y),
                width: f64::from(hole.rect.width),
                height: f64::from(hole.rect.height),
            },
        })
        .collect()
}

#[cfg(test)]
#[path = "holes_tests.rs"]
mod tests;
