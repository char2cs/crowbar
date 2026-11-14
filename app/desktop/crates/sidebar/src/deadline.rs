//! Where a removal hold stands.

use std::time::Instant;

/// A hold counts down to `At`, waits for the confirm dialog, or has been sent to the daemon.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Deadline {
    /// The delete is sent when the clock reaches this instant.
    At(Instant),
    /// No clock: the confirm dialog decides.
    AwaitsConfirmation,
    /// The delete was sent; the row stays hidden until the daemon's tombstone arrives.
    Committed,
}
