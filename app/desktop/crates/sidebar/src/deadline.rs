//! Where a removal hold stands.

use std::time::Instant;

/// The state of a removal hold: counting down, waiting for the user, or already sent.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Deadline {
    /// Counting down; the delete is sent at this instant.
    At(Instant),
    /// No countdown; the delete waits for the user to confirm.
    AwaitsConfirmation,
    /// The delete has been sent; the row stays hidden until it is gone.
    Committed,
}
