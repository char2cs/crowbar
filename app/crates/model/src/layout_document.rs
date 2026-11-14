use std::borrow::Cow;

use serde::{Deserialize, Serialize};

use crate::tab::Tab;
use crate::tab_id::TabId;

#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct LayoutDocument<'a> {
    pub(crate) version: u32,
    pub(crate) tabs: Cow<'a, [Tab]>,
    pub(crate) active: Option<TabId>,
}
