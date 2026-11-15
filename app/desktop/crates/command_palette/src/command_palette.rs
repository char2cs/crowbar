use crate::fuzzy_match::fuzzy_match;
use crate::palette_entry::PaletteEntry;
use crate::ranked_entry::RankedEntry;

/// The entries that match `query`, best first; equal scores keep their listed order. An empty
/// query keeps every entry in the listed order.
#[must_use]
pub fn rank(
    entries: &[PaletteEntry],
    query: &str,
) -> Vec<RankedEntry> {
    let mut hits: Vec<(i32, RankedEntry)> = entries
        .iter()
        .filter_map(|entry| rank_one(entry, query))
        .collect();
    hits.sort_by_key(|(score, _)| std::cmp::Reverse(*score));
    hits.into_iter().map(|(_, ranked)| ranked).collect()
}

fn rank_one(
    entry: &PaletteEntry,
    query: &str,
) -> Option<(i32, RankedEntry)> {
    let hit = fuzzy_match(query, &entry.title)?;
    let ranked = RankedEntry {
        entry: entry.clone(),
        positions: hit.positions,
    };
    Some((hit.score, ranked))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn entry(
        id: &str,
        title: &str,
    ) -> PaletteEntry {
        PaletteEntry {
            id: id.to_owned(),
            title: title.to_owned(),
            shortcut: None,
        }
    }

    fn ids(ranked: &[RankedEntry]) -> Vec<&str> {
        ranked.iter().map(|hit| hit.entry.id.as_str()).collect()
    }

    #[test]
    fn matches_come_best_first_and_non_matches_are_dropped() {
        let entries = [
            entry("far", "Toggle all bars"),
            entry("none", "Quit"),
            entry("near", "New tab"),
        ];
        const CASES: &[(&str, &[&str])] = &[
            ("", &["far", "none", "near"]),
            ("tab", &["near", "far"]),
            ("zzz", &[]),
        ];
        for (query, expected) in CASES {
            assert_eq!(ids(&rank(&entries, query)), *expected, "{query:?}");
        }
    }

    #[test]
    fn matches_carry_their_highlight_positions() {
        let ranked = rank(&[entry("a", "New tab")], "nt");
        assert_eq!(
            ranked.first().map(|hit| hit.positions.clone()),
            Some(vec![0, 4])
        );
    }
}
