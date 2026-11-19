use serde_json::Value;

use crate::error::{Error, Result};

const VERSION_KEY: &str = "version";

/// The version of the stored layout format this build writes, and the only one it reads.
pub const SCHEMA_VERSION: u32 = 3;

pub(crate) fn is_current(document: &Value) -> Result<bool> {
    let fields = document
        .as_object()
        .ok_or(Error::Invalid("layout is not an object"))?;
    let found = fields.get(VERSION_KEY);
    let current = found.and_then(Value::as_u64) == Some(u64::from(SCHEMA_VERSION));
    if !current {
        let found = found.map_or_else(|| "none".to_owned(), ToString::to_string);
        tracing::warn!(
            found,
            supported = SCHEMA_VERSION,
            "the stored layout has another schema version; starting from a default layout"
        );
    }
    Ok(current)
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn only_the_written_version_is_current() {
        let cases = [
            ("current", json!({"version": SCHEMA_VERSION}), true),
            ("unversioned", json!({"tabs": []}), false),
            ("older", json!({"version": SCHEMA_VERSION - 1}), false),
            ("newer", json!({"version": SCHEMA_VERSION + 1}), false),
            ("not a number", json!({"version": "one"}), false),
            ("negative", json!({"version": -1}), false),
        ];
        for (name, document, expected) in cases {
            assert_eq!(is_current(&document), Ok(expected), "{name}");
        }
    }

    #[test]
    fn a_document_that_is_not_an_object_is_corrupt() {
        for bad in [json!([]), json!("x"), json!(3)] {
            assert!(matches!(is_current(&bad), Err(Error::Invalid(_))), "{bad}");
        }
    }
}
