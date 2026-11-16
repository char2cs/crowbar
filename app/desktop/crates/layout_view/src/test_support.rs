//! Fixtures shared by the unit tests: layouts built through the model's own commands.
#![cfg(test)]

mod fake_source;
mod fixtures;
mod view_harness;

pub(crate) use fake_source::FakeSource;
pub(crate) use fixtures::{chat, layout_of, pane_id, stacked_layout};
pub(crate) use view_harness::{open, open_under_row_source, open_with};
