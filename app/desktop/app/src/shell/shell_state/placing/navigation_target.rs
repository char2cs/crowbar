use browser::PageUrl;
use serde_json::Value;
#[derive(Debug)]
pub(crate) struct NavigationTarget<'a> {
    pub(crate) url: &'a PageUrl,
    pub(crate) answer: Value,
}
