use serde_json::{Map, Value};

use crate::error::{Error, Result};

const VERSION_KEY: &str = "version";
const TABS_KEY: &str = "tabs";
const SPACE_KEY: &str = "space";
const SHOWN_KEY: &str = "shown";
const ACTIVE_KEY: &str = "active";
const NO_SPACE: &str = "";
const V3: u64 = 3;

pub(crate) const SCHEMA_VERSION: u32 = 4;

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
    fn only_the_current_or_version_3_is_read_and_a_non_object_is_corrupt() {
        let cases = [
            (json!({"version": SCHEMA_VERSION}), true),
            (json!({"version": 3, "tabs": [], "active": null}), true),
            (json!({"version": "one"}), false),
            (json!({"version": -1}), false),
        ];
        for (document, read) in cases {
            let answer = current(document.clone()).map(|d| d.is_some());
            assert_eq!(answer, Ok(read), "{document}");
        }
        let empty = json!({"version": 3, "active": null, "tabs": []});
        let shown = current(empty).unwrap().unwrap();
        assert_eq!(shown["shown"], json!({}), "a v3 without an active tab");
        for bad in [json!([]), json!("x"), json!(3)] {
            let refused = current(bad.clone());
            assert!(matches!(refused, Err(Error::Invalid(_))), "{bad}");
        }
    }
}
