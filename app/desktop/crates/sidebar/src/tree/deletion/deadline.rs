use std::time::Instant;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Deadline {
    At(Instant),
    AwaitsConfirmation,
    Committed,
}
