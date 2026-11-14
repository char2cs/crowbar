//! Draws one block of a page, or nothing when the search hides it.

use gpui::{AnyElement, App, IntoElement, Window};

use crate::block::Block;
use crate::button_block::button_block;
use crate::frame::Frame;
use crate::heading_view::heading_view;
use crate::list_view::list_view;
use crate::section_view::section_view;
use crate::shortcuts_view::shortcuts_view;

pub(crate) fn block_view(
    frame: &Frame<'_>,
    (index, block): (usize, &Block),
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    match block {
        Block::Section(section) => section_view(frame, section, (window, cx)),
        Block::Shortcuts(group) => shortcuts_view(frame, group, (window, cx)),
        _ if !frame.state.block_visible(index) => None,
        Block::Heading(heading) => Some(heading_view(heading, cx).into_any_element()),
        Block::List(list) => Some(list_view(frame, list, (window, cx)).into_any_element()),
        Block::Button(spec) => button_block(frame, spec).map(IntoElement::into_any_element),
    }
}

#[cfg(test)]
#[path = "block_view_tests.rs"]
mod tests;
