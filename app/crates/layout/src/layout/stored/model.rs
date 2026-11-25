use crate::error::{Error, Result};
use crate::layout::Layout;

/// Reads a layout from its stored JSON.
///
/// A document of another schema version, older or newer, loads as the default layout with a
/// warning; the result satisfies every [`Layout`] invariant, so a damaged file is an error rather
/// than a broken window.
///
/// # Errors
/// [`Error::Unreadable`] for text that is not a layout document and [`Error::Invalid`] for data
/// that breaks an invariant.
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
    use crate::layout::command::Command;
    use crate::layout::stored::schema::SCHEMA_VERSION;
    use crate::tab_id::TabId;

    fn layout() -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: "one".into(),
                space: crate::SpaceId::default(),
            })
            .unwrap();
        layout
    }

    #[test]
    fn a_stored_layout_loads_back_and_another_version_loads_as_the_default() {
        assert_eq!(
            load(&store(&layout()).unwrap()).unwrap(),
            layout(),
            "round trip"
        );
        let unversioned = json!({"tabs": [{"id": 1, "title": "one", "panes": {}, "tree": null, "focused": null}], "active": 1});
        let newer = json!({"version": SCHEMA_VERSION + 1, "tabs": 3});
        for other in [unversioned, newer] {
            let loaded = load(other.to_string().as_bytes()).unwrap();
            assert_eq!(loaded, Layout::default(), "{other}");
        }
    }

    #[test]
    fn unreadable_and_broken_data_are_distinct_errors() {
        let broken = json!({"version": SCHEMA_VERSION, "tabs": [], "shown": {"": 4}});
        let shape = json!({"version": SCHEMA_VERSION, "tabs": 3});
        let cases = [
            (
                "syntax",
                b"not json".to_vec(),
                Error::Unreadable(String::new()),
            ),
            (
                "shape",
                shape.to_string().into_bytes(),
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
}
