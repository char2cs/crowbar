pub(super) fn clamp_signed(
    asked: i64,
    wrap: usize,
    max: usize,
) -> usize {
    let from_end = asked + i64::from(asked < 0) * signed(wrap);
    usize::try_from(from_end).unwrap_or(0).clamp(1, max.max(1))
}

pub(super) fn signed(count: usize) -> i64 {
    i64::try_from(count).unwrap_or(i64::MAX)
}

pub(super) fn leading_number(part: &str) -> Option<i64> {
    let part = part.trim();
    let (sign, digits) = match part.strip_prefix('-') {
        Some(rest) => (-1, rest),
        None => (1, part.strip_prefix('+').unwrap_or(part)),
    };
    let digits: String = digits.chars().take_while(char::is_ascii_digit).collect();
    digits.parse::<i64>().ok().map(|number| sign * number)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_part_reads_its_leading_number_as_javascript_s_parse_int_does() {
        let cases = [
            ("12", Some(12)),
            (" -3x", Some(-3)),
            ("+4", Some(4)),
            ("x1", None),
            ("", None),
        ];
        for (part, expected) in cases {
            assert_eq!(leading_number(part), expected, "{part:?}");
        }
    }
}
