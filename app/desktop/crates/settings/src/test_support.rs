//! Fixtures shared by the unit tests: table cases and a state over the built-in schema.
#![cfg(test)]

mod cases;
mod state_fixtures;

pub(crate) use cases::{Case, run_cases};
pub(crate) use state_fixtures::{
    clear_button, custom_row, engine_row, gitignore_row, schema, size_row, state, state_with, write,
};
