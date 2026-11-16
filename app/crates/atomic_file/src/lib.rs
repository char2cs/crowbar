//! Replaces a file all-or-nothing, so a crash or a full disk never leaves it truncated.

mod atomic_file;

pub use atomic_file::write;
