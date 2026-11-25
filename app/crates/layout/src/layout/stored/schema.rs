use serde_json::{Map, Value};

use crate::error::{Error, Result};

const VERSION_KEY: &str = "version";
const TABS_KEY: &str = "tabs";
const SPACE_KEY: &str = "space";
const SHOWN_KEY: &str = "shown";
const ACTIVE_KEY: &str = "active";
const NO_SPACE: &str = "";
const V3: u64 = 3;

/// The version of the stored layout format this build writes. It reads this one and upgrades
/// version 3, whose tabs had no space; any other version loads as the default layout.
pub const SCHEMA_VERSION: u32 = 4;

pub(crate) fn current(mut document: Value) -> Result<Option<Value>> {
    let fields = document
        .as_object_mut()
        .ok_or(Error::Invalid("layout is not an object"))?;
    let found = fields.get(VERSION_KEY).and_then(Value::as_u64);
    if found == Some(V3) {
        upgrade_v3(fields);
    }
    if fields.get(VERSION_KEY).and_then(Value::as_u64) == Some(u64::from(SCHEMA_VERSION)) {
        return Ok(Some(document));
    }
    let found = fields
        .get(VERSION_KEY)
        .map_or_else(|| "none".to_owned(), ToString::to_string);
    tracing::warn!(
        found,
        supported = SCHEMA_VERSION,
        "the stored layout has another schema version; starting from a default layout"
    );
    Ok(None)
}

fn upgrade_v3(fields: &mut Map<String, Value>) {
    let active = fields.remove(ACTIVE_KEY).filter(|id| !id.is_null());
    let shown: Map<String, Value> = active
        .map(|id| (NO_SPACE.to_owned(), id))
        .into_iter()
        .collect();
    fields.insert(SHOWN_KEY.to_owned(), Value::Object(shown));
    let tabs = fields.get_mut(TABS_KEY).and_then(Value::as_array_mut);
    for tab in tabs.into_iter().flatten().filter_map(Value::as_object_mut) {
        tab.insert(SPACE_KEY.to_owned(), Value::from(NO_SPACE));
    }
    fields.insert(VERSION_KEY.to_owned(), Value::from(SCHEMA_VERSION));
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn the_written_version_is_current_and_version_3_is_upgraded() {
        let cases = [
            ("current", json!({"version": SCHEMA_VERSION}), true),
            (
                "version 3",
                json!({"version": 3, "tabs": [], "active": null}),
                true,
            ),
            ("unversioned", json!({"tabs": []}), false),
            ("older", json!({"version": 2}), false),
            ("newer", json!({"version": SCHEMA_VERSION + 1}), false),
            ("not a number", json!({"version": "one"}), false),
            ("negative", json!({"version": -1}), false),
        ];
        for (name, document, expected) in cases {
            assert_eq!(
                current(document).map(|d| d.is_some()),
                Ok(expected),
                "{name}"
            );
        }
    }

    #[test]
    fn a_version_3_document_gains_spaces_and_shows_its_active_tab_in_none() {
        let v3 = json!({"version": 3, "active": 7, "tabs": [{"id": 7, "title": "t"}]});
        let upgraded = current(v3).unwrap().unwrap();
        assert_eq!(
            upgraded,
            json!({"version": SCHEMA_VERSION, "shown": {"": 7}, "tabs": [{"id": 7, "title": "t", "space": ""}]}),
            "upgraded"
        );
        let empty = json!({"version": 3, "active": null, "tabs": []});
        assert_eq!(
            current(empty).unwrap().unwrap()["shown"],
            json!({}),
            "nothing shown"
        );
    }

    #[test]
    fn a_document_that_is_not_an_object_is_corrupt() {
        for bad in [json!([]), json!("x"), json!(3)] {
            assert!(
                matches!(current(bad.clone()), Err(Error::Invalid(_))),
                "{bad}"
            );
        }
    }
}
