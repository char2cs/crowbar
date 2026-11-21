//! The dialog frame: its size and the search field in the header.

use gpui::{App, Entity, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{DialogExtent, TextInput};

use crate::frame::Frame;
use crate::frame::spacing;
use crate::modal::page_picker::page_picker;

const NARROW: f32 = 720.0;
const NARROWEST: f32 = 520.0;
const SEARCH_WIDTH: f32 = 256.0;
const SEARCH_NARROW: f32 = 176.0;
const SEARCH_NARROWEST: f32 = 128.0;
const DIALOG_MAX_WIDTH: f32 = 1120.0;
const DIALOG_MAX_HEIGHT: f32 = 820.0;
const NARROW_SHARE: (f32, f32) = (1.0, 0.86);
const WIDE_SHARE: (f32, f32) = (0.9, 0.74);

fn viewport_width(window: &Window) -> f32 {
    f32::from(window.viewport_size().width)
}

pub(crate) fn extent(window: &Window) -> DialogExtent {
    let (width, height) = shares_for(viewport_width(window));
    DialogExtent::new(width, height, px(DIALOG_MAX_WIDTH), px(DIALOG_MAX_HEIGHT))
}

fn shares_for(width: f32) -> (f32, f32) {
    if width < NARROW {
        return NARROW_SHARE;
    }
    WIDE_SHARE
}

fn search_width(width: f32) -> f32 {
    if width < NARROWEST {
        return SEARCH_NARROWEST;
    }
    if width < NARROW {
        return SEARCH_NARROW;
    }
    SEARCH_WIDTH
}

pub(crate) fn is_narrow(window: &Window) -> bool {
    viewport_width(window) < NARROW
}

pub(crate) fn header_actions(
    frame: &Frame<'_>,
    input: &Entity<TextInput>,
    (window, cx): (&Window, &App),
) -> impl IntoElement {
    let search = div()
        .w(px(search_width(viewport_width(window))))
        .child(input.clone());
    let picker = is_narrow(window).then(|| page_picker(frame, cx));
    div()
        .flex()
        .flex_1()
        .min_w_0()
        .items_center()
        .justify_between()
        .gap(spacing::COMFORTABLE)
        .children(picker)
        .child(search)
}

#[cfg(test)]
mod tests {
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
}
