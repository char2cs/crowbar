use serde_json::{Map, Value};

use crate::error::{Error, Result};

/// The version of the stored layout format this build writes.
pub const SCHEMA_VERSION: u32 = 1;

type Migration = fn(&mut Map<String, Value>);

const MIGRATIONS: [Migration; 1] = [stamp_first_version];

fn stamp_first_version(document: &mut Map<String, Value>) {
    document.insert("version".to_owned(), Value::from(1));
}

fn version_of(document: &Map<String, Value>) -> Result<u32> {
    let Some(found) = document.get("version") else {
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
    if found > SCHEMA_VERSION {
        return Err(Error::UnsupportedSchema {
            found,
            supported: SCHEMA_VERSION,
        });
    }
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

    #[test]
    fn an_unversioned_document_is_stamped_with_the_first_version() {
        let upgraded = upgrade(json!({"tabs": [], "active": null})).unwrap();
        assert_eq!(
            upgraded,
            json!({"version": 1, "tabs": [], "active": null}),
            "migrated"
        );
    }

    #[test]
    fn a_current_document_is_left_alone() {
        let current = json!({"version": SCHEMA_VERSION, "tabs": [], "active": null});
        assert_eq!(upgrade(current.clone()).unwrap(), current, "unchanged");
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
        for bad in [
            json!([]),
            json!("x"),
            json!({"version": "one"}),
            json!({"version": -1}),
            json!({"version": 1.5}),
        ] {
            assert!(
                matches!(upgrade(bad.clone()), Err(Error::Invalid(_))),
                "{bad}"
            );
        }
    }
}
