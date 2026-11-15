//! Builds terminals with known content for the unit tests.
#![cfg(test)]

mod fixtures;

pub(crate) use fixtures::{
    FOREGROUND, ansi, empty_terminal, emulator_showing, row_text, terminal_showing,
};
