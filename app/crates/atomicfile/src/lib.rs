//! Replaces a file all-or-nothing, so a crash or a full disk never leaves it truncated.

mod adapters;
mod atomicfile;

pub use atomicfile::write;
