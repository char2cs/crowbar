//! A removal in progress: the row hides behind a countdown until the delete is sent.

use std::time::{Duration, Instant};

use crate::deadline::Deadline;
use crate::removal_kind::RemovalKind;
use crate::row_id::RowId;

const MILLIS_PER_SECOND: u128 = 1000;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Hold {
    pub(crate) row: RowId,
    pub(crate) kind: RemovalKind,
    pub(crate) label: String,
    pub(crate) deadline: Deadline,
}

impl Hold {
    pub(crate) fn seconds_left(
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

    pub(crate) fn is_due(
        &self,
        now: Instant,
    ) -> bool {
        matches!(self.deadline, Deadline::At(at) if at <= now)
    }

    pub(crate) fn drain(
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
