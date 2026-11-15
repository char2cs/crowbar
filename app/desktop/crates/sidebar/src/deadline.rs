//! Where a removal hold stands.

use std::time::Instant;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Deadline {
    At(Instant),
    AwaitsConfirmation,
    Committed,
}
