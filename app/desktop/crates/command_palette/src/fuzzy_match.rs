const CONSECUTIVE_BONUS: i32 = 15;
const WORD_START_BONUS: i32 = 8;
const FIRST_CHAR_BONUS: i32 = 2;
const GAP_PENALTY: i32 = 1;

/// How well a query fits a text.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FuzzyMatch {
    /// Higher is better; only comparable between matches of the same query.
    pub score: i32,
    /// Character positions of the text the query letters landed on.
    pub positions: Vec<usize>,
}

/// Matches `query` against `text` as a case-insensitive subsequence. Letters that follow each
/// other and letters that start a word score higher, gaps cost a little. `None` when some
/// query letter has no place left in the text; an empty query matches everything with score 0.
#[must_use]
pub fn fuzzy_match(
    query: &str,
    text: &str,
) -> Option<FuzzyMatch> {
    let wanted: Vec<char> = query
        .chars()
        .filter(|c| !c.is_whitespace())
        .flat_map(char::to_lowercase)
        .collect();
    let chars: Vec<char> = text.chars().collect();
    let lowered: Vec<char> = chars.iter().flat_map(|c| c.to_lowercase().next()).collect();
    let end = leftmost_end(&wanted, &lowered)?;
    let start = tightest_start(&wanted, &lowered, end);
    let positions = forward_positions(&wanted, &lowered, start);
    Some(FuzzyMatch {
        score: score(&positions, &chars),
        positions,
    })
}

fn leftmost_end(
    wanted: &[char],
    lowered: &[char],
) -> Option<usize> {
    let mut at = 0;
    let mut last = None;
    for letter in wanted {
        let found = lowered.get(at..)?.iter().position(|c| c == letter)?;
        last = Some(at + found);
        at += found + 1;
    }
    last.or(Some(0))
}

fn tightest_start(
    wanted: &[char],
    lowered: &[char],
    end: usize,
) -> usize {
    let mut at = end + 1;
    let mut start = 0;
    for letter in wanted.iter().rev() {
        let head = lowered.get(..at).unwrap_or(lowered);
        start = head.iter().rposition(|c| c == letter).unwrap_or(0);
        at = start;
    }
    start
}

fn forward_positions(
    wanted: &[char],
    lowered: &[char],
    start: usize,
) -> Vec<usize> {
    let mut at = start;
    let mut positions = Vec::with_capacity(wanted.len());
    for letter in wanted {
        let found = lowered
            .get(at..)
            .and_then(|tail| tail.iter().position(|c| c == letter))
            .map_or(at, |offset| at + offset);
        positions.push(found);
        at = found + 1;
    }
    positions
}

fn score(
    positions: &[usize],
    chars: &[char],
) -> i32 {
    let mut total = 0;
    let mut previous: Option<usize> = None;
    for &position in positions {
        total += letter_score(position, previous, chars);
        previous = Some(position);
    }
    total
}

fn letter_score(
    position: usize,
    previous: Option<usize>,
    chars: &[char],
) -> i32 {
    let adjacent = previous.is_some_and(|before| before + 1 == position);
    let word_start = position == 0
        || chars
            .get(position - 1)
            .is_some_and(|c| !c.is_alphanumeric());
    let mut points = 0;
    if adjacent {
        points += CONSECUTIVE_BONUS;
    }
    if word_start {
        points += WORD_START_BONUS;
    }
    if position == 0 {
        points += FIRST_CHAR_BONUS;
    }
    let gap = previous.map_or(position, |before| position.saturating_sub(before + 1));
    points - i32::try_from(gap).unwrap_or(i32::MAX) * GAP_PENALTY
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn query_letters_land_in_order_ignoring_case_and_spaces() {
        const CASES: &[(&str, &str, Option<&[usize]>)] = &[
            ("nt", "New tab", Some(&[0, 4])),
            ("NEW T", "New tab", Some(&[0, 1, 2, 4])),
            ("ab", "a xxxx ab", Some(&[7, 8])),
            ("", "anything", Some(&[])),
            ("tn", "New tab", None),
            ("xyz", "New tab", None),
            ("aa", "a", None),
        ];
        for (query, text, expected) in CASES {
            let positions = fuzzy_match(query, text).map(|hit| hit.positions);
            assert_eq!(positions.as_deref(), *expected, "{query} in {text}");
        }
    }

    #[test]
    fn the_tighter_match_scores_higher_and_an_empty_query_scores_zero() {
        let tight = fuzzy_match("tab", "New tab").unwrap();
        let loose = fuzzy_match("tab", "Toggle all bars").unwrap();
        assert!(tight.score > loose.score);
        assert_eq!(fuzzy_match("", "anything").map(|hit| hit.score), Some(0));
    }
}
