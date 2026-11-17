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

impl ReplyScope {
    pub(crate) fn arm(
        &mut self,
        pane: &str,
    ) {
        if let Self::LoadEnd {
            pane: waiting,
            armed,
            ..
        } = self
            && waiting == pane
        {
            *armed = true;
        }
    }

    pub(crate) fn is_armed_for(
        &self,
        pane: &str,
    ) -> bool {
        matches!(self, Self::LoadEnd { pane: waiting, armed: true, .. } if waiting == pane)
    }

    pub(crate) fn waits_on_closed(
        &self,
        is_open: &impl Fn(&str) -> bool,
    ) -> bool {
        matches!(self, Self::LoadEnd { pane, .. } if !is_open(pane))
    }
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
    fn a_load_wait_is_armed_only_by_its_own_panes_next_load_and_ends_with_its_pane() {
        let mut scope = ReplyScope::LoadEnd {
            pane: "b1".into(),
            armed: false,
            result: json!({}),
        };
        scope.arm("b2");
        assert!(!scope.is_armed_for("b1"), "another pane's load");
        scope.arm("b1");
        assert!(scope.is_armed_for("b1") && !scope.is_armed_for("b2"));
        assert!(scope.waits_on_closed(&|_| false) && !scope.waits_on_closed(&|_| true));
        assert!(!ReplyScope::Shell.waits_on_closed(&|_| false));
    }

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
