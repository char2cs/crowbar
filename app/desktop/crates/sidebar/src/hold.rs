//! A removal in progress: the row hides behind a countdown until the delete is sent.

use std::time::{Duration, Instant};

use crate::deadline::Deadline;
use crate::removal_kind::RemovalKind;
use crate::row_id::RowId;

/// One entry of the removal tray.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Hold {
    /// The row being removed.
    pub row: RowId,
    /// What it is.
    pub kind: RemovalKind,
    /// The text of the row, for the confirm dialog.
    pub label: String,
    /// Where the removal stands.
    pub deadline: Deadline,
}

impl Hold {
    /// Whole seconds left, rounded up; `None` when no clock runs.
    pub fn seconds_left(&self, now: Instant) -> Option<u64> {
        let Deadline::At(deadline) = self.deadline else {
            return None;
        };
        let left = deadline.saturating_duration_since(now);
        Some(
            left.as_millis()
                .div_ceil(1000)
                .try_into()
                .unwrap_or(u64::MAX),
        )
    }

    pub(crate) fn is_due(&self, now: Instant) -> bool {
        matches!(self.deadline, Deadline::At(at) if at <= now)
    }

    pub(crate) fn drain(&self, now: Instant, total: Duration) -> f32 {
        let Deadline::At(deadline) = self.deadline else {
            return 0.0;
        };
        let left = deadline.saturating_duration_since(now).as_secs_f32();
        (left / total.as_secs_f32()).clamp(0.0, 1.0)
    }
}

#[cfg(test)]
#[path = "hold_tests.rs"]
mod tests;
