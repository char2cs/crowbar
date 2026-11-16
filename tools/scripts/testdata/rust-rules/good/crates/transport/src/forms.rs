pub(crate) fn sum_pairs<T: Into<u32>>(
    (first, second): (u32, u32),
    extra: T,
) -> u32 {
    // SPEC: docs/rabbyte-manifest.md#i-code a tuple pattern counts as two parameters
    // PLATFORM: macos needs this sum on the main thread
    first + second + extra.into()
}

/// Joins the names of the given items.
pub fn join_names(items: &[&str]) -> Vec<String> {
    items.iter().map(|item| item.to_uppercase()).collect()
}

/// Tells whether the name is empty.
pub fn is_blank(name: &str) -> bool {
    name.is_empty()
}

/// Counts the items the predicate accepts.
pub fn count_where(
    items: &[u32],
    keep: impl Fn(u32) -> bool,
) -> usize {
    items.iter().filter(|item| keep(**item)).count()
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: &[((u32, u32), u32)] = &[((1, 2), 3), ((2, 2), 4), ((0, 0), 0)];

    #[test]
    fn sums_every_case() {
        for (pair, want) in CASES {
            assert_eq!(sum_pairs(*pair, 0u32), *want);
        }
    }

    #[test]
    fn adds_the_extra() {
        assert_eq!(sum_pairs((1, 1), 1u32), 3);
    }

    #[test]
    fn predicates_are_not_flags() {
        assert!(is_blank(""));
        assert_eq!(count_where(&[1, 2], |item| item > 1), 1);
    }

    #[test]
    fn joins_names() {
        assert_eq!(join_names(&["a"]), vec!["A".to_owned()]);
    }
}
