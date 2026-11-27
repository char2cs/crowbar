const PREFIX: char = '\u{1}';
const OVERSIZED: &str = "oversized:";
const DROPPED: &str = "dropped";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum IpcNotice {
    Oversized(usize),
    Dropped,
}

impl IpcNotice {
    pub(crate) fn encode(self) -> String {
        match self {
            Self::Oversized(bytes) => format!("{PREFIX}{OVERSIZED}{bytes}"),
            Self::Dropped => format!("{PREFIX}{DROPPED}"),
        }
    }

    pub(crate) fn parse(text: &str) -> Option<Self> {
        let body = text.strip_prefix(PREFIX)?;
        if body == DROPPED {
            return Some(Self::Dropped);
        }
        body.strip_prefix(OVERSIZED)?
            .parse()
            .ok()
            .map(Self::Oversized)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_notice_survives_its_text_form_and_page_text_is_never_one() {
        let cases = [
            (IpcNotice::Oversized(2_000_000).encode(), true),
            (IpcNotice::Dropped.encode(), true),
            (r#"{"kind":"pane.setTitle"}"#.to_owned(), false),
            ("\u{1}oversized:many".to_owned(), false),
        ];
        for (text, is_notice) in cases {
            assert_eq!(IpcNotice::parse(&text).is_some(), is_notice, "{text:?}");
        }
        assert_eq!(
            IpcNotice::parse(&IpcNotice::Oversized(7).encode()),
            Some(IpcNotice::Oversized(7)),
            "size"
        );
    }
}
