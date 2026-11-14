//! A titled group of rows.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};

use crate::frame::Frame;
use crate::row::Row;
use crate::row_view::row_view;
use crate::section::Section;
use crate::title_block::title_block;

pub(crate) fn section_view(
    frame: &Frame<'_>,
    section: &Section,
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    let rows: Vec<&Row> = section
        .rows
        .iter()
        .filter(|row| frame.state.row_visible(row))
        .collect();
    if rows.is_empty() {
        return None;
    }
    let header = section
        .show_header
        .then(|| title_block(&section.title, section.description.as_deref(), cx));
    let mut body: Vec<AnyElement> = Vec::with_capacity(rows.len());
    for row in rows {
        body.push(row_view(frame, row, (&mut *window, &mut *cx)).into_any_element());
    }
    let section = div()
        .px(px(4.0))
        .py(px(2.0))
        .children(header)
        .child(div().flex().flex_col().gap(px(8.0)).children(body));
    Some(section.into_any_element())
}

#[cfg(test)]
#[path = "section_view_tests.rs"]
mod tests;
