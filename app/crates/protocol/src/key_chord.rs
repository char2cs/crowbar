use crate::error::{Error, Result};
use crate::string_id::{check_text, string_id};

const MAX_CHORD_BYTES: usize = 64;

fn validate(value: &str) -> Result<()> {
    if value.len() > MAX_CHORD_BYTES {
        return Err(Error::IdTooLong {
            kind: "key chord",
            max: MAX_CHORD_BYTES,
        });
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
#[path = "key_chord_tests.rs"]
mod tests;
