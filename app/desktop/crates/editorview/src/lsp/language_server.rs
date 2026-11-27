use gpui::{App, Task};
use serde_json::Value;

use crate::LspError;

pub trait LanguageServer {
    fn post(
        &self,
        verb: &'static str,
        body: Value,
        cx: &mut App,
    ) -> Task<Result<Value, LspError>>;
}
