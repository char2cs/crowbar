//! The brackets Monaco's bracket pair colorization paints: those outside strings and comments.

use std::ops::Range;

const BRACKETS: &[u8] = b"()[]{}";

/// The byte offsets of the brackets of `text` outside the `quiet` ranges, in order.
#[must_use]
pub fn brackets(
    text: &str,
    quiet: &[Range<usize>],
) -> Vec<usize> {
    let quiet = merged(quiet);
    text.bytes()
        .enumerate()
        .filter(|(offset, byte)| BRACKETS.contains(byte) && !is_quiet(&quiet, *offset))
        .map(|(offset, _)| offset)
        .collect()
}

fn merged(quiet: &[Range<usize>]) -> Vec<Range<usize>> {
    let mut sorted = quiet.to_vec();
    sorted.sort_unstable_by_key(|range| range.start);
    sorted.into_iter().fold(Vec::new(), |mut merged, range| {
        match merged.last_mut() {
            Some(last) if range.start <= last.end => last.end = last.end.max(range.end),
            _ => merged.push(range),
        }
        merged
    })
}

fn is_quiet(
    quiet: &[Range<usize>],
    offset: usize,
) -> bool {
    let after = quiet.partition_point(|range| range.start <= offset);
    after
        .checked_sub(1)
        .and_then(|at| quiet.get(at))
        .is_some_and(|range| range.contains(&offset))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn brackets_in_quiet_ranges_are_left_alone() {
        assert_eq!(brackets("f(\"(\") // )", &[2..5, 7..11, 3..4]), [1, 5]);
    }
}
