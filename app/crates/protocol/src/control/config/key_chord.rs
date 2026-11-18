use crate::error::{Error, Result};
use crate::string_id::{check_text, string_id};

const MAX_CHORD_BYTES: usize = 64;

fn validate(value: &str) -> Result<()> {
    if value.len() > MAX_CHORD_BYTES {
        return Err(Error::too_long("key chord", MAX_CHORD_BYTES));
    }
    check_text("key chord", value, |c| c.is_ascii_graphic())
}

string_id!(
    /// A key chord in the shell's notation, for example `mod+k` or `mod+shift+,`: printable
    /// ASCII without spaces, at most 64 bytes.
    KeyChord,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn is_printable_ascii_without_spaces() {
        for ok in ["mod+k", "mod+shift+,", "f5"] {
            assert!(KeyChord::new(ok).is_ok(), "{ok}");
        }
        for bad in ["", "mod k", "mod+\u{e9}", "a\tb"] {
            assert!(KeyChord::new(bad).is_err(), "{bad:?}");
        }
        assert!(KeyChord::new("k".repeat(65)).is_err(), "too long");
    }
}
