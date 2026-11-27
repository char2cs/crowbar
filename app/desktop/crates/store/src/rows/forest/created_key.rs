const FRACTION_DIGITS: usize = 9;

pub(crate) fn created_key(stamp: &str) -> String {
    let (head, rest) = stamp.split_once('.').unwrap_or((stamp, ""));
    let head = head.trim_end_matches('Z');
    let digits: String = rest.chars().take_while(char::is_ascii_digit).collect();
    format!("{head}.{digits:0<FRACTION_DIGITS$}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn stamps_of_different_precision_compare_by_instant() {
        let earlier = created_key("2026-11-15T02:13:33.05Z");
        let later = created_key("2026-11-15T02:13:33.050047789Z");
        assert!(earlier < later);
        assert_eq!(
            created_key("2026-11-15T02:13:33Z"),
            created_key("2026-11-15T02:13:33.0Z")
        );
        assert_eq!(created_key(""), ".000000000");
    }
}
