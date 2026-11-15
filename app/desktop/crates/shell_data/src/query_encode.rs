//! Percent-encoding of a value in a route's query string.

fn keeps(byte: u8) -> bool {
    byte.is_ascii_alphanumeric() || b"-._~/".contains(&byte)
}

fn escape(byte: u8) -> String {
    if keeps(byte) {
        return char::from(byte).to_string();
    }
    format!("%{byte:02X}")
}

pub(crate) fn encode(value: &str) -> String {
    value.bytes().map(escape).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str); 5] = [
        ("/", "/"),
        ("/srv/app-1_x.y~z", "/srv/app-1_x.y~z"),
        ("/my docs", "/my%20docs"),
        ("/a&b=c#d?", "/a%26b%3Dc%23d%3F"),
        ("/é", "/%C3%A9"),
    ];

    #[test]
    fn everything_but_unreserved_bytes_and_slashes_is_escaped() {
        for (raw, encoded) in CASES {
            assert_eq!(encode(raw), encoded, "{raw}");
        }
    }
}
