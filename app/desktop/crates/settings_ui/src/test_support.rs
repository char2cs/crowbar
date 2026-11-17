//! Fakes shared by the unit tests: a modal over a fake backend and a window that measures one element.
#![cfg(test)]

mod cases;
mod harness;
mod probe;
mod state_fixtures;

pub(crate) use cases::{Case, run_cases};
pub(crate) use harness::{Harness, backend, inspect, open, row, run};
pub(crate) use probe::measure;
pub(crate) use state_fixtures::{
    clear_button, custom_row, engine_row, gitignore_row, schema, size_row, state, state_with, write,
};
