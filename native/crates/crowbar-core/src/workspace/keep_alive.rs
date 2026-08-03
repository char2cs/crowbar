//! Pure retention policy for workspace keep-alive (spec §4 / P3).
//!
//! Ported from `web/src/features/workspace/lib/keep-alive-policy.ts`.
//!
//! Given the mounted workspaces and their last-active timestamps, decides
//! which to keep in memory and which to destroy, plus when the next
//! timer-driven eviction should run. There is no clock read here — `now` is
//! injected so the function stays deterministic and unit-testable; the host
//! owns the clock.
//!
//! Invariants (unchanged from the TS source):
//!
//! * The most-recently-active workspace is **always** retained. The host
//!   refreshes the active workspace's timestamp to `now` before every call, so
//!   "most recent" is the active workspace — this is what guarantees the
//!   active workspace is never evicted, even on the timer path when it has sat
//!   idle past the window.
//! * A non-active workspace is retained while its age (`now - last_active_at`)
//!   is strictly less than `window_ms`. The boundary (age `== window_ms`)
//!   counts as expired, so a timer armed for exactly that instant evicts it
//!   instead of re-arming for the same instant forever.
//! * At most `cap` workspaces are retained (xterm WebGL context ceiling). When
//!   more than `cap` are within the window, the oldest are evicted regardless.
//! * `window_ms == 0` reproduces destroy-on-switch: only the active workspace
//!   survives.

use std::collections::HashSet;

/// Hard cap on simultaneously-retained workspaces (xterm WebGL contexts).
pub const RETENTION_CAP: usize = 6;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RetentionEntry {
    pub ws_id: String,
    pub last_active_at: i64,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct RetentionPlan {
    /// Workspace ids to keep mounted, in the input's order.
    pub retain: Vec<String>,
    /// Workspace ids to destroy now, in the input's order.
    pub evict: Vec<String>,
    /// Epoch ms at which the next timer-driven eviction should run, or `None`
    /// when nothing is on a timer (no non-active retained entries, or
    /// `window_ms == 0`).
    pub next_expiry_at: Option<i64>,
}

#[must_use]
pub fn plan_retention(
    entries: &[RetentionEntry],
    now: i64,
    window_ms: i64,
    cap: usize,
) -> RetentionPlan {
    if entries.is_empty() {
        return RetentionPlan::default();
    }

    // The most-recently-active entry is the protected (active) workspace. On a
    // tie, the earliest entry wins — a strict `>` comparison, same as the TS
    // `reduce` starting from the first element.
    let mut most_recent = 0usize;
    for (index, entry) in entries.iter().enumerate().skip(1) {
        if entry.last_active_at > entries[most_recent].last_active_at {
            most_recent = index;
        }
    }

    let has_window = window_ms > 0;
    let is_live = |index: usize, entry: &RetentionEntry| -> bool {
        index == most_recent || (has_window && now - entry.last_active_at < window_ms)
    };

    // Candidate retained set: the active workspace plus every in-window entry.
    let mut candidates: Vec<usize> = entries
        .iter()
        .enumerate()
        .filter(|(index, entry)| is_live(*index, entry))
        .map(|(index, _)| index)
        .collect();

    // Enforce the hard cap by recency (most-recent first; input order breaks
    // ties). The active workspace has the max timestamp, so it always
    // survives the truncation below.
    candidates.sort_by(|&a, &b| {
        entries[b]
            .last_active_at
            .cmp(&entries[a].last_active_at)
            .then(a.cmp(&b))
    });
    candidates.truncate(cap.max(1));

    let retain_indexes: HashSet<usize> = candidates.iter().copied().collect();

    let mut retain = Vec::new();
    let mut evict = Vec::new();
    for (index, entry) in entries.iter().enumerate() {
        if retain_indexes.contains(&index) {
            retain.push(entry.ws_id.clone());
        } else {
            evict.push(entry.ws_id.clone());
        }
    }

    // The next eviction fires when the earliest-expiring RETAINED non-active
    // entry crosses the window. The active workspace never expires.
    let mut next_expiry_at: Option<i64> = None;
    if has_window {
        for &index in &candidates {
            if index == most_recent {
                continue;
            }
            let expiry = entries[index].last_active_at + window_ms;
            next_expiry_at = Some(next_expiry_at.map_or(expiry, |current| current.min(expiry)));
        }
    }

    RetentionPlan {
        retain,
        evict,
        next_expiry_at,
    }
}

#[cfg(test)]
mod tests {
    use super::{RETENTION_CAP, RetentionEntry, RetentionPlan, plan_retention};

    const MIN: i64 = 60_000;

    fn entry(ws_id: &str, last_active_at: i64) -> RetentionEntry {
        RetentionEntry {
            ws_id: ws_id.to_string(),
            last_active_at,
        }
    }

    // --- ported from web/src/__tests__/features/workspace/lib/keep-alive-policy.test.ts ---

    #[test]
    fn retains_a_lone_active_workspace_and_schedules_nothing() {
        let plan = plan_retention(&[entry("a", 1000)], 1000, 10 * MIN, RETENTION_CAP);
        assert_eq!(plan.retain, vec!["a".to_string()]);
        assert!(plan.evict.is_empty());
        assert_eq!(plan.next_expiry_at, None);
    }

    #[test]
    fn returns_empty_plan_for_no_entries() {
        let plan = plan_retention(&[], 5000, 10 * MIN, RETENTION_CAP);
        assert_eq!(plan, RetentionPlan::default());
    }

    #[test]
    fn retains_entries_whose_age_is_within_the_window() {
        let now = 100_000;
        let plan = plan_retention(
            &[entry("active", now), entry("recent", now - 2 * MIN)],
            now,
            10 * MIN,
            RETENTION_CAP,
        );
        assert_eq!(
            plan.retain,
            vec!["active".to_string(), "recent".to_string()]
        );
        assert!(plan.evict.is_empty());
        assert_eq!(plan.next_expiry_at, Some(now - 2 * MIN + 10 * MIN));
    }

    #[test]
    fn evicts_entries_whose_age_exceeds_the_window_keeping_the_active_one() {
        let now = 100_000;
        let plan = plan_retention(
            &[entry("active", now), entry("stale", now - 11 * MIN)],
            now,
            10 * MIN,
            RETENTION_CAP,
        );
        assert_eq!(plan.retain, vec!["active".to_string()]);
        assert_eq!(plan.evict, vec!["stale".to_string()]);
        assert_eq!(plan.next_expiry_at, None);
    }

    #[test]
    fn treats_an_entry_exactly_at_the_window_boundary_as_expired() {
        let now = 100_000;
        let plan = plan_retention(
            &[entry("active", now), entry("edge", now - 10 * MIN)],
            now,
            10 * MIN,
            RETENTION_CAP,
        );
        assert_eq!(plan.retain, vec!["active".to_string()]);
        assert_eq!(plan.evict, vec!["edge".to_string()]);
    }

    #[test]
    fn never_evicts_the_most_recently_active_workspace_even_when_idle_past_the_window() {
        let now = 100_000;
        let plan = plan_retention(
            &[entry("active", now - 30 * MIN)],
            now,
            10 * MIN,
            RETENTION_CAP,
        );
        assert_eq!(plan.retain, vec!["active".to_string()]);
        assert!(plan.evict.is_empty());
        assert_eq!(plan.next_expiry_at, None);
    }

    #[test]
    fn window_ms_zero_retains_only_the_active_workspace() {
        let now = 100_000;
        let plan = plan_retention(
            &[
                entry("active", now),
                entry("b", now - 1),
                entry("c", now - 2),
            ],
            now,
            0,
            RETENTION_CAP,
        );
        assert_eq!(plan.retain, vec!["active".to_string()]);
        assert_eq!(plan.evict, vec!["b".to_string(), "c".to_string()]);
        assert_eq!(plan.next_expiry_at, None);
    }

    #[test]
    fn caps_the_retained_set_at_the_hard_cap_evicting_the_oldest_despite_the_window() {
        let now = 1_000_000;
        // 7 workspaces all within the window; cap is 6, so the oldest is evicted.
        let entries: Vec<RetentionEntry> = (0..7i64)
            .map(|i| entry(&format!("ws{i}"), now - (6 - i) * 1000))
            .collect();
        let plan = plan_retention(&entries, now, 60 * MIN, 6);
        assert_eq!(
            plan.retain,
            vec!["ws1", "ws2", "ws3", "ws4", "ws5", "ws6"]
                .into_iter()
                .map(String::from)
                .collect::<Vec<_>>()
        );
        assert_eq!(plan.evict, vec!["ws0".to_string()]);
    }

    #[test]
    fn preserves_input_order_in_retain_and_evict_arrays() {
        let now = 1000;
        let plan = plan_retention(
            &[
                entry("x", now - 20 * MIN), // stale
                entry("y", now),            // active
                entry("z", now - MIN),      // recent
            ],
            now,
            10 * MIN,
            RETENTION_CAP,
        );
        assert_eq!(plan.retain, vec!["y".to_string(), "z".to_string()]);
        assert_eq!(plan.evict, vec!["x".to_string()]);
    }

    #[test]
    fn next_expiry_at_is_the_earliest_retained_non_active_expiry() {
        let now = 500_000;
        let plan = plan_retention(
            &[
                entry("active", now),
                entry("older", now - 5 * MIN),
                entry("newer", now - MIN),
            ],
            now,
            10 * MIN,
            RETENTION_CAP,
        );
        assert_eq!(plan.next_expiry_at, Some(now - 5 * MIN + 10 * MIN));
    }

    #[test]
    fn exposes_a_hard_cap_of_6() {
        assert_eq!(RETENTION_CAP, 6);
    }

    // --- new: tie-breaking and cap-edge cases the TS suite didn't exercise ---

    #[test]
    fn ties_at_the_max_timestamp_favour_the_earliest_entry_as_active() {
        // Two entries share the max timestamp; the TS `reduce`'s strict `>`
        // keeps the first (lowest-index) one as "most recent", so on a
        // window_ms of 0 the SECOND one — despite an identical timestamp — is
        // not the protected entry and gets evicted.
        let now = 1000;
        let plan = plan_retention(
            &[entry("first", now), entry("second", now)],
            now,
            0,
            RETENTION_CAP,
        );
        assert_eq!(plan.retain, vec!["first".to_string()]);
        assert_eq!(plan.evict, vec!["second".to_string()]);
    }

    #[test]
    fn a_cap_of_zero_still_retains_at_least_the_active_workspace() {
        // Math.max(1, cap) in the TS source — a caller passing 0 must not
        // evict the active workspace.
        let now = 1000;
        let plan = plan_retention(&[entry("active", now)], now, 10 * MIN, 0);
        assert_eq!(plan.retain, vec!["active".to_string()]);
    }
}
