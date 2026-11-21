//! The dialog frame: its size and the search field in the header.

use gpui::{App, Entity, InteractiveElement, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{Dialog, DialogExtent, HeaderFlow, TextInput};

use crate::frame::Frame;
use crate::modal::page_picker::page_picker;

const TITLE: &str = "Settings";
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
    ui::app_window_width(window)
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

pub(crate) fn search_field(
    input: &Entity<TextInput>,
    window: &Window,
) -> impl IntoElement {
    div()
        .debug_selector(|| "settings-search".to_owned())
        .w(px(search_width(viewport_width(window))))
        .child(input.clone())
}

pub(crate) fn headed(
    dialog: Dialog,
    frame: &Frame<'_>,
    (window, cx): (&Window, &App),
) -> Dialog {
    if !is_narrow(window) {
        return dialog.title(TITLE);
    }
    dialog
        .heading(page_picker(frame, cx))
        .header_flow(HeaderFlow::Grid)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::open;

    #[gpui::test]
    fn under_720px_the_close_drops_under_the_picker_left_of_the_search(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "appearance");
        for width in [640.0, 600.0] {
            cx.simulate_resize(gpui::size(px(width), px(480.0)));
            cx.run_until_parked();
            let search = cx.debug_bounds("settings-search").unwrap_or_default();
            let close = cx.debug_bounds("dialog-close").unwrap_or_default();
            assert_eq!(
                close.top(),
                search.bottom() + px(8.0),
                "{width}: {close:?} {search:?}"
            );
            assert_eq!(
                close.right(),
                search.left() - px(16.0),
                "{width}: {close:?}"
            );
            assert_eq!(
                close.size.height, search.size.height,
                "{width}: buttons grow with inputs"
            );
        }
    }

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
