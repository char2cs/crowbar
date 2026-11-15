use crate::error::{Error, Result};
use crate::string_id::string_id;

const KIND: &str = "path";
const MAX_PATH_BYTES: usize = 4096;

fn validate(value: &str) -> Result<()> {
    if value.is_empty() {
        return Err(Error::EmptyId { kind: KIND });
    }
    if value.len() > MAX_PATH_BYTES {
        return Err(Error::IdTooLong {
            kind: KIND,
            max: MAX_PATH_BYTES,
        });
    }
    if let Some(found) = value.chars().find(|c| c.is_control()) {
        return Err(Error::InvalidIdCharacter { kind: KIND, found });
    }
    if !value.starts_with('/') {
        return Err(Error::MalformedId {
            kind: KIND,
            reason: "must be absolute",
        });
    }
    Ok(())
}

string_id!(
    /// An absolute file system path (`/...`) a pane asks the shell to act on, at most 4096 bytes
    /// and free of control characters. Windows paths are not supported.
    FsPath,
    validate
);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_absolute_paths_without_control_characters() {
        for ok in ["/", "/home/a b/\u{e9}.txt"] {
            assert!(FsPath::new(ok).is_ok(), "{ok}");
        }
        let long = format!("/{}", "x".repeat(MAX_PATH_BYTES));
        for bad in ["", "relative/a", "~/a", "/a\nb", "/a\0b", long.as_str()] {
            assert!(FsPath::new(bad).is_err(), "{bad:?}");
        }
    }
}
