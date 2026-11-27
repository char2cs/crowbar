use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::error::{Error, Result};

pub(crate) fn typed<T: DeserializeOwned>(
    path: &str,
    data: Value,
) -> Result<T> {
    let empty_list = Some(&data)
        .filter(|data| data.is_null())
        .and_then(|_| serde_json::from_value(Value::Array(Vec::new())).ok());
    if let Some(list) = empty_list {
        return Ok(list);
    }
    serde_json::from_value(data).map_err(|source| Error::Json {
        path: path.into(),
        source,
    })
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn data_parses_as_the_asked_type_or_names_the_route() {
        assert_eq!(typed::<Vec<u8>>("/x", json!([1, 2])).unwrap(), [1, 2]);
        let failed = typed::<Vec<u8>>("/x", json!("no")).unwrap_err();
        assert!(failed.to_string().contains("/x"));
    }

    #[test]
    fn a_null_answer_is_an_empty_list_for_a_list_and_none_for_an_option() {
        assert_eq!(
            typed::<Vec<u8>>("/x", Value::Null).unwrap(),
            Vec::<u8>::new()
        );
        assert_eq!(typed::<Option<u8>>("/x", Value::Null).unwrap(), None);
        assert!(typed::<u8>("/x", Value::Null).is_err());
    }
}
