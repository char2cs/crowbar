use browser::PageUrl;
use serde_json::Value;
#[derive(Debug)]
pub(super) struct NavigationTarget<'a> {
    pub(super) url: &'a PageUrl,
    pub(super) answer: Value,
}
