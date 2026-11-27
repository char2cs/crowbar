use crate::error::{Error, Result};
use crate::string_id::string_id;

const KIND: &str = "document path";
const MAX_PATH_BYTES: usize = 4096;
const PARENT: &str = "..";

fn validate(value: &str) -> Result<()> {
    if value.is_empty() {
        return Err(Error::EmptyId { kind: KIND });
    }
    if value.len() > MAX_PATH_BYTES {
        return Err(Error::too_long(KIND, MAX_PATH_BYTES));
    }
    if let Some(found) = value.chars().find(|c| c.is_control()) {
        return Err(Error::InvalidIdCharacter { kind: KIND, found });
    }
    if value.split('/').any(|segment| segment == PARENT) {
        return Err(Error::malformed(KIND, "must not climb out with .."));
    }
    Ok(())
}

string_id!(DocumentPath, validate);

impl DocumentPath {
    #[must_use]
    pub fn is_host(&self) -> bool {
        self.as_str().starts_with('/')
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn names_a_workspace_or_host_file_that_stays_where_it_says() {
        let cases = [
            ("src/a b.ts", Some(false)),
            ("/etc/hosts", Some(true)),
            ("README.md", Some(false)),
            ("", None),
            ("../secret", None),
            ("src/../../x", None),
            ("a\nb", None),
        ];
        for (text, host) in cases {
            let parsed = DocumentPath::new(text).ok().map(|path| path.is_host());
            assert_eq!(parsed, host, "{text:?}");
        }
        let long = "x".repeat(MAX_PATH_BYTES + 1);
        assert!(DocumentPath::new(long).is_err(), "too long");
    }
}
