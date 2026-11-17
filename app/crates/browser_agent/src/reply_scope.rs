use serde_json::Value;

#[derive(Debug)]
pub(crate) enum ReplyScope {
    Shell,
    Page(String),
    LoadEnd {
        pane: String,
        armed: bool,
        result: Value,
    },
}

pub(crate) fn merged(
    result: Value,
    page: &Value,
) -> Value {
    match (result, page) {
        (Value::Object(mut fields), Value::Object(shown)) => {
            fields.extend(
                shown
                    .iter()
                    .map(|(key, value)| (key.clone(), value.clone())),
            );
            Value::Object(fields)
        },
        (result, _) => result,
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn the_page_adds_to_an_object_result_and_never_replaces_a_plain_one() {
        let cases = [
            (
                json!({"tab":"b1","url":"old"}),
                json!({"url":"new"}),
                json!({"tab":"b1","url":"new"}),
            ),
            (json!("closed"), json!({"url":"new"}), json!("closed")),
            (json!({"tab":"b1"}), json!(null), json!({"tab":"b1"})),
        ];
        for (result, page, expected) in cases {
            assert_eq!(merged(result, &page), expected);
        }
    }
}
