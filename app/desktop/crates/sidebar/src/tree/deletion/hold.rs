//! A removal in progress: the row hides behind a countdown until the delete is sent.

use std::time::{Duration, Instant};

use crate::shell::RowId;
use crate::tree::deletion::deadline::Deadline;
use crate::tree::deletion::removal_kind::RemovalKind;

const MILLIS_PER_SECOND: u128 = 1000;

/// One row held back from deletion until its countdown ends or the user confirms.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Hold {
    /// The row being removed.
    pub row: RowId,
    /// What the removal deletes.
    pub kind: RemovalKind,
    /// The row's name, shown in the countdown and the confirmation.
    pub label: String,
    /// Where the hold stands.
    pub deadline: Deadline,
}

impl Hold {
    /// The whole seconds left on the countdown, rounded up.
    /// `None` when the hold has no clock: it awaits confirmation or is committed.
    pub fn seconds_left(
        &self,
        now: Instant,
    ) -> Option<u64> {
        let Deadline::At(deadline) = self.deadline else {
            return None;
        };
        let left = deadline.saturating_duration_since(now);
        Some(
            left.as_millis()
                .div_ceil(MILLIS_PER_SECOND)
                .try_into()
                .unwrap_or(u64::MAX),
        )
    }

    /// The title of the notice saying the daemon refused to remove the row, naming it.
    pub fn refusal_title(&self) -> String {
        format!("Couldn't remove {}", self.label)
    }

    pub(crate) fn is_due(
        &self,
        now: Instant,
    ) -> bool {
        matches!(self.deadline, Deadline::At(at) if at <= now)
    }

    /// The fraction of the countdown still left, from `1.0` at the start to `0.0` when due.
    /// A hold without a clock reads `0.0`.
    pub fn drain(
        &self,
        now: Instant,
        total: Duration,
    ) -> f32 {
        let Deadline::At(deadline) = self.deadline else {
            return 0.0;
        };
        let left = deadline.saturating_duration_since(now).as_secs_f32();
        (left / total.as_secs_f32()).clamp(0.0, 1.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hold_at(deadline: Deadline) -> Hold {
        Hold {
            row: RowId::from("c"),
            kind: RemovalKind::Chat,
            label: "chat".into(),
            deadline,
        }
    }

    type SecondCase = (&'static str, Option<u64>, u64, Option<u64>);

    const SECOND_CASES: &[SecondCase] = &[
        ("7.001s shows 8", Some(7_001), 0, Some(8)),
        ("due shows 0", Some(7_001), 7_001, Some(0)),
        ("confirm has no clock", None, 0, None),
    ];

    fn deadline_after(
        now: Instant,
        millis: Option<u64>,
    ) -> Deadline {
        millis.map_or(Deadline::AwaitsConfirmation, |ms| {
            Deadline::At(now + Duration::from_millis(ms))
        })
    }

    #[test]
    fn seconds_round_up_and_clockless_holds_have_none() {
        let now = Instant::now();
        for (name, deadline_ms, at_ms, expected) in SECOND_CASES {
            let deadline = deadline_after(now, *deadline_ms);
            let at = now + Duration::from_millis(*at_ms);
            assert_eq!(hold_at(deadline).seconds_left(at), *expected, "{name}");
        }
    }

    const DUE_CASES: &[(&str, u64, bool)] = &[("not yet", 0, false), ("due", 1, true)];

    #[test]
    fn due_only_at_or_after_the_deadline() {
        let now = Instant::now();
        let hold = hold_at(Deadline::At(now + Duration::from_secs(1)));
        for (name, at_secs, expected) in DUE_CASES {
            let at = now + Duration::from_secs(*at_secs);
            assert_eq!(hold.is_due(at), *expected, "{name}");
        }
    }

    #[test]
    fn a_refusal_is_titled_by_the_row() {
        let hold = hold_at(Deadline::Committed);
        assert_eq!(hold.refusal_title(), "Couldn't remove chat");
    }

    #[test]
    fn the_drain_fraction_tracks_the_real_deadline() {
        let now = Instant::now();
        let hold = hold_at(Deadline::At(now + Duration::from_secs(2)));
        let total = Duration::from_secs(8);
        assert!(
            (hold.drain(now, total) - 0.25).abs() < 0.001,
            "2s of 8s left"
        );
        assert!(
            hold.drain(now + Duration::from_secs(5), total).abs() < f32::EPSILON,
            "empty after the deadline"
        );
    }
}
