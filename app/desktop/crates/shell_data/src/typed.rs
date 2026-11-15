//! Reading the `data` of a daemon answer as the type a route promises.

use serde::de::DeserializeOwned;
use serde_json::Value;

use crate::error::{Error, Result};

pub(crate) fn typed<T: DeserializeOwned>(
    path: &str,
    data: Value,
) -> Result<T> {
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
}
