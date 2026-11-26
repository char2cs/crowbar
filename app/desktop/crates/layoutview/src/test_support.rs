//! Fixtures shared by the unit tests: layouts built through the model's own commands.
#![cfg(test)]

mod fake_source;
mod fixtures;
mod view_harness;

pub(crate) use fake_source::{FakeSource, PREVIEW, PREVIEW_HEIGHT, PREVIEW_WIDTH};
pub(crate) use fixtures::{
    chat, editor, layout_of, open_tab, pane_id, stacked_layout, strip_layout,
};
pub(crate) use view_harness::{
    Floating, chat_row, file_row, open, open_floating, open_floating_with, open_under_row_source,
    open_with, strip_of,
};
