//! The rows hidden behind a removal countdown.

use std::time::{Duration, Instant};

use crate::frame::REMOVAL_DRAIN_SECONDS;
use crate::shell::RowId;
use crate::tree::deletion::deadline::Deadline;
use crate::tree::deletion::hold::Hold;
use crate::tree::deletion::removal_kind::RemovalKind;

/// The removal holds in progress, at most one per row.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct RemovalTray {
    holds: Vec<Hold>,
}

impl RemovalTray {
    /// How long a countdown runs before the delete is sent.
    pub const DRAIN: Duration = Duration::from_secs(REMOVAL_DRAIN_SECONDS);

    pub(crate) fn hold(
        &mut self,
        row: RowId,
        (kind, label): (RemovalKind, String),
        now: Instant,
    ) {
        self.holds.retain(|hold| hold.row != row);
        let deadline = Self::deadline_for(kind, now);
        self.holds.push(Hold {
            row,
            kind,
            label,
            deadline,
        });
    }

    fn deadline_for(
        kind: RemovalKind,
        now: Instant,
    ) -> Deadline {
        if kind.needs_confirmation() {
            return Deadline::AwaitsConfirmation;
        }
        Deadline::At(now + Self::DRAIN)
    }

    pub(crate) fn get(
        &self,
        row: &RowId,
    ) -> Option<&Hold> {
        self.holds.iter().find(|hold| &hold.row == row)
    }

    /// Drop the row's hold so it shows again and nothing is sent.
    pub fn keep(
        &mut self,
        row: &RowId,
    ) {
        self.holds.retain(|hold| &hold.row != row);
    }

    /// The rows whose countdown has ended by `now`, marked committed so each is returned once.
    pub fn take_due(
        &mut self,
        now: Instant,
    ) -> Vec<RowId> {
        self.holds
            .iter_mut()
            .filter(|hold| hold.is_due(now))
            .map(|hold| {
                hold.deadline = Deadline::Committed;
                hold.row.clone()
            })
            .collect()
    }

    /// Mark the row's hold committed: its delete has been sent and it stays hidden until gone.
    pub fn commit(
        &mut self,
        row: &RowId,
    ) {
        for hold in self.holds.iter_mut().filter(|hold| &hold.row == row) {
            hold.deadline = Deadline::Committed;
        }
    }

    pub(crate) fn await_confirmation(
        &mut self,
        row: &RowId,
    ) {
        for hold in self.holds.iter_mut().filter(|hold| &hold.row == row) {
            hold.deadline = Deadline::AwaitsConfirmation;
        }
    }

    /// The earliest countdown deadline, so one timer serves every hold; `None` when none counts.
    pub fn next_deadline(&self) -> Option<Instant> {
        self.holds
            .iter()
            .filter_map(|hold| match hold.deadline {
                Deadline::At(at) => Some(at),
                _ => None,
            })
            .min()
    }

    pub(crate) fn release_gone(
        &mut self,
        mut exists: impl FnMut(&RowId) -> bool,
    ) {
        self.holds.retain(|hold| exists(&hold.row));
    }

    /// Whether any hold is still counting down.
    pub fn is_counting(&self) -> bool {
        self.holds
            .iter()
            .any(|hold| matches!(hold.deadline, Deadline::At(_)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn chat(id: &str) -> (RowId, (RemovalKind, String)) {
        (RowId::from(id), (RemovalKind::Chat, id.to_owned()))
    }

    type HoldCase = (&'static str, RemovalKind, Option<u64>);

    const HOLD_CASES: &[HoldCase] = &[
        ("chats count down eight seconds", RemovalKind::Chat, Some(8)),
        ("repos wait for confirmation", RemovalKind::Repo, None),
    ];

    #[test]
    fn holds_count_down_or_wait_for_confirmation() {
        let now = Instant::now();
        for (name, kind, seconds) in HOLD_CASES {
            let mut tray = RemovalTray::default();
            tray.hold(RowId::from("x"), (*kind, "x".into()), now);
            let hold = tray.get(&RowId::from("x")).expect("held");
            assert_eq!(hold.seconds_left(now), *seconds, "{name}");
        }
    }

    #[test]
    fn one_timer_serves_every_countdown() {
        let now = Instant::now();
        let mut tray = RemovalTray::default();
        let (id, info) = chat("c");
        tray.hold(id, info, now);
        tray.hold(RowId::from("r"), (RemovalKind::Repo, "demo".into()), now);
        assert_eq!(tray.next_deadline(), Some(now + RemovalTray::DRAIN));
    }

    #[test]
    fn keep_restores_the_row_and_due_holds_are_sent_once() {
        let now = Instant::now();
        let mut tray = RemovalTray::default();
        for id in ["a", "b"] {
            let (row, info) = chat(id);
            tray.hold(row, info, now);
        }
        tray.keep(&RowId::from("a"));
        assert!(
            tray.get(&RowId::from("a")).is_none(),
            "kept row is no longer held"
        );
        assert!(tray.take_due(now).is_empty(), "nothing due yet");
        let due = tray.take_due(now + RemovalTray::DRAIN);
        assert_eq!(due, vec![RowId::from("b")], "b is sent");
        assert!(
            tray.take_due(now + RemovalTray::DRAIN).is_empty(),
            "never sent twice"
        );
        assert!(!tray.is_counting(), "no countdown left");
    }

    #[test]
    fn tombstones_release_committed_holds() {
        let now = Instant::now();
        let mut tray = RemovalTray::default();
        let (row, info) = chat("c");
        tray.hold(row.clone(), info, now);
        tray.commit(&row);
        tray.release_gone(|id| id.as_str() != "c");
        assert!(tray.get(&row).is_none(), "released when the row is gone");
    }
}
