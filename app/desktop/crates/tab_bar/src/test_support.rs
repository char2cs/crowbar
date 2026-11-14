//! Fixtures shared by the unit tests: a strip in a test window that records what it asks for.
#![cfg(test)]

mod harness;

pub(crate) use harness::{open, view};
