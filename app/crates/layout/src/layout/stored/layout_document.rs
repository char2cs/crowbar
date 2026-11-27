use std::borrow::Cow;
use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

use crate::layout::tab::Tab;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;

#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct LayoutDocument<'a> {
    pub(crate) version: u32,
    pub(crate) tabs: Cow<'a, [Tab]>,
    pub(crate) shown: Cow<'a, BTreeMap<SpaceId, TabId>>,
}
