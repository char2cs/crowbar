use browser::BrowserUrl;
use serde_json::Value;
#[derive(Debug)]
pub(super) struct NavigationTarget<'a> {
    pub(super) url: &'a BrowserUrl,
    pub(super) answer: Value,
}
