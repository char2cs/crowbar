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
    fn a_stored_layout_loads_back_identical() {
        let bytes = store(&layout()).unwrap();
        assert_eq!(load(&bytes).unwrap(), layout(), "round trip");
    }

    #[test]
    fn the_first_unversioned_format_is_migrated_on_load() {
        let old = json!({"tabs": [{"id": 1, "title": "one", "panes": {}, "tree": null, "focused": null}], "active": 1});
        assert_eq!(
            load(old.to_string().as_bytes()).unwrap(),
            layout(),
            "migrated"
        );
    }

    #[test]
    fn unreadable_newer_and_broken_data_are_distinct_errors() {
        assert!(
            matches!(load(b"not json"), Err(Error::Unreadable(_))),
            "syntax"
        );
        assert!(
            matches!(load(b"{\"tabs\": 3}"), Err(Error::Unreadable(_))),
            "shape"
        );
        let newer = json!({"version": SCHEMA_VERSION + 1, "tabs": [], "active": null});
        assert_eq!(
            load(newer.to_string().as_bytes()),
            Err(Error::UnsupportedSchema {
                found: SCHEMA_VERSION + 1,
                supported: SCHEMA_VERSION
            }),
            "newer"
        );
        let broken = json!({"version": 1, "tabs": [], "active": 4});
        assert!(
            matches!(load(broken.to_string().as_bytes()), Err(Error::Invalid(_))),
            "invariant"
        );
    }
}
