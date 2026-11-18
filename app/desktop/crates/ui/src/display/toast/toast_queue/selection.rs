//! Which toasts a column shows when it has room for only a few.

use crate::display::toast::toast_queue::entry::Entry;

pub(crate) fn visible(
    entries: &[Entry],
    limit: Option<usize>,
) -> Vec<&Entry> {
    let Some(limit) = limit else {
        return entries.iter().collect();
    };
    let pinned = entries.iter().filter(|entry| entry.is_pinned()).count();
    let transient = entries.iter().filter(|entry| !entry.is_pinned());
    let shown: Vec<_> = transient
        .take(limit.saturating_sub(pinned))
        .map(|entry| entry.id)
        .collect();
    entries
        .iter()
        .filter(|entry| entry.is_pinned() || shown.contains(&entry.id))
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::display::toast::toast_kind::ToastKind;
    use crate::display::toast::toast_queue::toast_id::ToastId;
    use crate::display::toast::toast_queue::toast_lifetime::ToastLifetime;

    fn entries(lifetimes: &[ToastLifetime]) -> Vec<Entry> {
        lifetimes
            .iter()
            .zip(0..)
            .map(|(&lifetime, id)| Entry {
                id: ToastId(id),
                key: None,
                kind: ToastKind::Info,
                message: "m".into(),
                lifetime,
                updates: 0,
                leaving: false,
                timer: None,
            })
            .collect()
    }

    #[test]
    fn pinned_toasts_are_always_kept_and_transient_ones_fill_the_rest_newest_first() {
        use ToastLifetime::{Auto, Pinned};
        type Case = (&'static [ToastLifetime], Option<usize>, &'static [u64]);
        const CASES: &[Case] = &[
            (&[Auto, Auto, Auto, Auto, Auto], Some(3), &[0, 1, 2]),
            (&[Auto, Pinned, Auto, Auto], Some(3), &[0, 1, 2]),
            (&[Auto, Auto, Pinned, Pinned, Auto], Some(3), &[0, 2, 3]),
            (
                &[Pinned, Pinned, Pinned, Pinned, Auto],
                Some(3),
                &[0, 1, 2, 3],
            ),
            (&[Auto, Auto, Auto, Auto, Auto], None, &[0, 1, 2, 3, 4]),
        ];
        for &(lifetimes, limit, want) in CASES {
            let held = entries(lifetimes);
            let got: Vec<u64> = visible(&held, limit)
                .iter()
                .map(|entry| entry.id.0)
                .collect();
            assert_eq!(got, want, "{lifetimes:?} {limit:?}");
        }
    }
}
