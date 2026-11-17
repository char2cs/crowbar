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
