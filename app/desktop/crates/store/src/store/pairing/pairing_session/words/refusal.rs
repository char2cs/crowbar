const PREFIX: &str = "the host refused pairing (";
const STATUS_DIGITS: usize = 3;

pub(crate) struct Refusal<'a> {
    pub(crate) status: &'a str,
    pub(crate) reason: &'a str,
}

impl<'a> Refusal<'a> {
    pub(crate) fn of(text: &'a str) -> Option<Self> {
        let rest = text.strip_prefix(PREFIX)?;
        let (inside, rest) = rest.split_once(')')?;
        let status = inside.get(..STATUS_DIGITS).unwrap_or(inside);
        let reason = rest.strip_prefix(": ")?;
        Some(Self { status, reason })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Read<'a> = Option<(&'a str, &'a str)>;

    const CASES: [(&str, Read<'static>); 3] = [
        (
            "the host refused pairing (401 Unauthorized): bad code",
            Some(("401", "bad code")),
        ),
        ("the host refused pairing (500)", None),
        ("cannot reach box:3737", None),
    ];

    #[test]
    fn a_refusal_reads_as_its_status_and_reason() {
        for (text, expected) in CASES {
            let read = Refusal::of(text).map(|found| (found.status, found.reason));
            assert_eq!(read, expected, "{text}");
        }
    }
}
