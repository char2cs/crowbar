//! Where the settings of the previous desktop app come from.

use std::collections::BTreeMap;

use crate::error::Result;

pub(crate) trait LegacyStore {
    /// Every `localStorage` entry of the old app, key to JSON text.
    fn entries(&self) -> Result<BTreeMap<String, String>>;
}
