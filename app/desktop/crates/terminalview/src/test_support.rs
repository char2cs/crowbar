//! A terminal view in a test window wired to a fake daemon peer.
#![cfg(test)]

mod harness;
mod pointer_events;

pub(crate) use harness::{
    cell_position, draw, feed, first_row, focus, open, record_events, sent, typed,
};
pub(crate) use pointer_events::{press, release};
