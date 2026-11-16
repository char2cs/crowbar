use serde_json::{Map, Value};

use crate::error::{Error, Result};

const VERSION_KEY: &str = "version";

/// The version of the stored layout format this build writes.
pub const SCHEMA_VERSION: u32 = 2;

type Migration = fn(&mut Map<String, Value>);

const MIGRATIONS: [Migration; 2] = [stamp_first_version, leaves_become_groups];

fn stamp_first_version(document: &mut Map<String, Value>) {
    document.insert(VERSION_KEY.to_owned(), Value::from(1));
}

fn leaves_become_groups(document: &mut Map<String, Value>) {
    let trees = document
        .get_mut("tabs")
        .and_then(Value::as_array_mut)
        .into_iter()
        .flatten()
        .filter_map(|tab| tab.get_mut("tree"));
    for tree in trees {
        group_leaves(tree);
    }
    document.insert(VERSION_KEY.to_owned(), Value::from(2));
}

fn group_leaves(node: &mut Value) {
    let Some(fields) = node.as_object_mut() else {
        return;
    };
    if let Some(first) = fields.get_mut("first") {
        group_leaves(first);
    }
    if let Some(second) = fields.get_mut("second") {
        group_leaves(second);
    }
    let Some(pane) = fields.remove("pane") else {
        return;
    };
    fields.insert("panes".to_owned(), Value::Array(vec![pane.clone()]));
    fields.insert("active".to_owned(), pane);
}

fn version_of(document: &Map<String, Value>) -> Result<u32> {
    let Some(found) = document.get(VERSION_KEY) else {
        return Ok(0);
    };
    found
        .as_u64()
        .and_then(|version| u32::try_from(version).ok())
        .ok_or(Error::Invalid("schema version is not a number"))
}

pub(crate) fn upgrade(mut value: Value) -> Result<Value> {
    let document = value
        .as_object_mut()
        .ok_or(Error::Invalid("layout is not an object"))?;
    let found = version_of(document)?;
    (found <= SCHEMA_VERSION)
        .then_some(())
        .ok_or(Error::UnsupportedSchema {
            found,
            supported: SCHEMA_VERSION,
        })?;
    let skip = usize::try_from(found).unwrap_or(usize::MAX);
    for migration in MIGRATIONS.iter().skip(skip) {
        migration(document);
    }
    Ok(value)
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    fn old_leaf(pane: &str) -> Value {
        json!({"node": "leaf", "pane": pane})
    }

    fn grouped_leaf(pane: &str) -> Value {
        json!({"node": "leaf", "panes": [pane], "active": pane})
    }

    fn nested(leaf: fn(&str) -> Value) -> Value {
        let inner = json!({"node": "split", "axis": "vertical", "ratio": 0.4,
            "first": leaf("b"), "second": leaf("c")});
        json!({"node": "split", "axis": "horizontal", "ratio": 0.5,
            "first": leaf("a"), "second": inner})
    }

    fn tabbed(
        version: u32,
        tree: &Value,
    ) -> Value {
        json!({"version": version, "tabs": [{"tree": tree}], "active": null})
    }

    #[test]
    fn documents_are_brought_to_the_current_version() {
        let current = json!({"version": SCHEMA_VERSION, "tabs": [], "active": null});
        let cases = [
            (
                "unversioned is brought to the current version",
                json!({"tabs": [], "active": null}),
                json!({"version": SCHEMA_VERSION, "tabs": [], "active": null}),
            ),
            (
                "version one leaves become groups of one",
                tabbed(1, &nested(old_leaf)),
                tabbed(2, &nested(grouped_leaf)),
            ),
            (
                "an empty tab has no tree to migrate",
                json!({"version": 1, "tabs": [{"tree": null}], "active": null}),
                json!({"version": 2, "tabs": [{"tree": null}], "active": null}),
            ),
            ("current is left alone", current.clone(), current),
        ];
        for (name, input, expected) in cases {
            assert_eq!(upgrade(input).unwrap(), expected, "{name}");
        }
    }

    #[test]
    fn a_newer_document_is_refused_rather_than_misread() {
        assert_eq!(
            upgrade(json!({"version": SCHEMA_VERSION + 1, "tabs": []})),
            Err(Error::UnsupportedSchema {
                found: SCHEMA_VERSION + 1,
                supported: SCHEMA_VERSION
            }),
            "newer"
        );
    }

    #[test]
    fn malformed_documents_are_refused() {
        let cases = [
            json!([]),
            json!("x"),
            json!({"version": "one"}),
            json!({"version": -1}),
            json!({"version": 1.5}),
        ];
        for bad in cases {
            assert!(
                matches!(upgrade(bad.clone()), Err(Error::Invalid(_))),
                "{bad}"
            );
        }
    }
}
