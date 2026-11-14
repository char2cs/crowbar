//! The rows hidden behind a removal countdown.

use std::time::{Duration, Instant};

use crate::deadline::Deadline;
use crate::hold::Hold;
use crate::metrics::REMOVAL_DRAIN_SECONDS;
use crate::removal_kind::RemovalKind;
use crate::row_id::RowId;

/// Every removal in flight. The deleting itself is the daemon's; the tray only remembers which
/// rows are waiting, until when, and which have been sent.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct RemovalTray {
    holds: Vec<Hold>,
}

impl RemovalTray {
    /// The span a countdown takes.
    pub const DRAIN: Duration = Duration::from_secs(REMOVAL_DRAIN_SECONDS);

    /// Holds `row`: a countdown from `now` for workspaces, folders and chats, no clock for
    /// repositories and spaces.
    pub fn hold(&mut self, row: RowId, (kind, label): (RemovalKind, String), now: Instant) {
        self.holds.retain(|hold| hold.row != row);
        let deadline = if kind.needs_confirmation() {
            Deadline::AwaitsConfirmation
        } else {
            Deadline::At(now + Self::DRAIN)
        };
        self.holds.push(Hold {
            row,
            kind,
            label,
            deadline,
        });
    }

    /// The hold of `row`, if it has one.
    pub fn get(&self, row: &RowId) -> Option<&Hold> {
        self.holds.iter().find(|hold| &hold.row == row)
    }

    /// Cancels the hold: the row reappears exactly as it was.
    pub fn keep(&mut self, row: &RowId) {
        self.holds.retain(|hold| &hold.row != row);
    }

    /// Marks every countdown that reached `now` as sent and returns their rows.
    pub fn take_due(&mut self, now: Instant) -> Vec<RowId> {
        let mut due = Vec::new();
        for hold in self.holds.iter_mut().filter(|hold| hold.is_due(now)) {
            hold.deadline = Deadline::Committed;
            due.push(hold.row.clone());
        }
        due
    }

    /// Marks the hold as sent, for a confirmed deletion.
    pub fn commit(&mut self, row: &RowId) {
        for hold in self.holds.iter_mut().filter(|hold| &hold.row == row) {
            hold.deadline = Deadline::Committed;
        }
    }

    /// Turns a countdown into a wait for the confirm dialog, after the daemon refused the delete.
    pub fn await_confirmation(&mut self, row: &RowId) {
        for hold in self.holds.iter_mut().filter(|hold| &hold.row == row) {
            hold.deadline = Deadline::AwaitsConfirmation;
        }
    }

    /// The soonest running deadline, for the one timer.
    pub fn next_deadline(&self) -> Option<Instant> {
        self.holds
            .iter()
            .filter_map(|hold| match hold.deadline {
                Deadline::At(at) => Some(at),
                _ => None,
            })
            .min()
    }

    /// Forgets holds whose row the daemon no longer lists: the tombstone arrived.
    pub fn release_gone(&mut self, mut exists: impl FnMut(&RowId) -> bool) {
        self.holds.retain(|hold| exists(&hold.row));
    }

    /// Whether any countdown is still running, so the view keeps animating.
    pub fn is_counting(&self) -> bool {
        self.holds
            .iter()
            .any(|hold| matches!(hold.deadline, Deadline::At(_)))
    }

    /// All holds.
    pub fn holds(&self) -> &[Hold] {
        &self.holds
    }
}

#[cfg(test)]
#[path = "removal_tray_tests.rs"]
mod tests;
