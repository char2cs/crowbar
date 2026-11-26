//! What a log event says about itself.

use std::fmt::Display;

use crate::Level;

/// One thing worth logging, as a value. Its `Display` is the line's message with its details.
pub trait Event: Display {
    /// The module the event speaks for, such as `supervisor::health`.
    fn source(&self) -> &'static str;

    /// The event's stable name, such as `health_unanswered`.
    fn name(&self) -> &'static str;

    /// How loud the event is.
    fn level(&self) -> Level;
}
