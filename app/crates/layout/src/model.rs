use crate::error::{Error, Result};
use crate::layout::Layout;

/// Reads a layout from its stored JSON, whatever version wrote it.
///
/// Older formats are migrated, newer ones refused, and the result satisfies every [`Layout`]
/// invariant, so a damaged file is an error rather than a broken window.
///
/// # Errors
/// [`Error::Unreadable`] for text that is not a layout document,
/// [`Error::UnsupportedSchema`] for a version newer than this build reads and
/// [`Error::Invalid`] for data that breaks an invariant.
pub fn load(stored: &[u8]) -> Result<Layout> {
    let value =
        serde_json::from_slice(stored).map_err(|source| Error::Unreadable(source.to_string()))?;
    Layout::from_stored(value)
}

/// Writes a layout as JSON in the current format ([`SCHEMA_VERSION`](crate::SCHEMA_VERSION)).
///
/// # Errors
/// [`Error::Unreadable`] when the layout cannot be encoded.
pub fn store(layout: &Layout) -> Result<Vec<u8>> {
    serde_json::to_vec(layout).map_err(|source| Error::Unreadable(source.to_string()))
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::command::Command;
    use crate::schema::SCHEMA_VERSION;
    use crate::tab_id::TabId;

    fn layout() -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: "one".into(),
            })
            .unwrap();
        layout
    }

    #[test]
    fn stored_and_legacy_documents_load_as_the_same_layout() {
        let legacy = json!({"tabs": [{"id": 1, "title": "one", "panes": {}, "tree": null, "focused": null}], "active": 1});
        let cases = [
            ("round trip", store(&layout()).unwrap()),
            (
                "unversioned format migrated",
                legacy.to_string().into_bytes(),
            ),
        ];
        for (name, bytes) in cases {
            assert_eq!(load(&bytes).unwrap(), layout(), "{name}");
        }
    }

    #[test]
    fn unreadable_and_broken_data_are_distinct_errors() {
        let broken = json!({"version": 1, "tabs": [], "active": 4});
        let cases = [
            (
                "syntax",
                b"not json".to_vec(),
                Error::Unreadable(String::new()),
            ),
            (
                "shape",
                b"{\"tabs\": 3}".to_vec(),
                Error::Unreadable(String::new()),
            ),
            (
                "invariant",
                broken.to_string().into_bytes(),
                Error::Invalid(""),
            ),
        ];
        for (name, bytes, kind) in cases {
            let error = load(&bytes).unwrap_err();
            assert_eq!(
                std::mem::discriminant(&error),
                std::mem::discriminant(&kind),
                "{name}"
            );
        }
    }

    #[test]
    fn newer_data_is_refused_with_the_supported_version() {
        let newer = json!({"version": SCHEMA_VERSION + 1, "tabs": [], "active": null});
        assert_eq!(
            load(newer.to_string().as_bytes()),
            Err(Error::UnsupportedSchema {
                found: SCHEMA_VERSION + 1,
                supported: SCHEMA_VERSION
            }),
            "newer"
        );
    }
}
