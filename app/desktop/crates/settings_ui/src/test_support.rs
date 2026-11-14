//! Fakes shared by the unit tests: a modal over a fake backend and a window that measures one element.
#![cfg(test)]

mod harness;
mod probe;

pub(crate) use harness::{Harness, backend, inspect, open, row, run};
pub(crate) use probe::measure;
