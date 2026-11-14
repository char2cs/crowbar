//! The dialog frame: its size and the search field in the header.

use gpui::{Entity, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{DialogExtent, TextInput};

const NARROW: f32 = 720.0;
const NARROWEST: f32 = 520.0;
const SEARCH_WIDTH: f32 = 256.0;
const SEARCH_NARROW: f32 = 176.0;
const SEARCH_NARROWEST: f32 = 128.0;

pub(crate) fn extent(window: &Window) -> DialogExtent {
    if f32::from(window.viewport_size().width) < NARROW {
        return DialogExtent::new(1.0, 0.86, px(1120.0), px(820.0));
    }
    DialogExtent::new(0.9, 0.74, px(1120.0), px(820.0))
}

pub(crate) fn search_field(input: &Entity<TextInput>, window: &Window) -> impl IntoElement {
    let width = f32::from(window.viewport_size().width);
    let pixels = if width < NARROWEST {
        SEARCH_NARROWEST
    } else if width < NARROW {
        SEARCH_NARROW
    } else {
        SEARCH_WIDTH
    };
    div().w(px(pixels)).child(input.clone())
}

#[cfg(test)]
#[path = "header_tests.rs"]
mod tests;
