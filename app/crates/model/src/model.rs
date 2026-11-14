use crate::error::{Error, Result};
use crate::layout::Layout;

/// Reads a layout from its stored JSON, whatever version wrote it.
///
/// Older formats are migrated, newer ones refused, and the result satisfies every [`Layout`]
/// invariant, so a damaged file is an error rather than a broken window.
///
/// # Errors
/// [`Error::Unreadable`] for text that is not a layout document,
/// [`Error::UnsupportedSchema`] for a version newer than this build reads and
/// [`Error::Invalid`] for data that breaks an invariant.
pub fn load(stored: &[u8]) -> Result<Layout> {
    let value =
        serde_json::from_slice(stored).map_err(|source| Error::Unreadable(source.to_string()))?;
    Layout::from_stored(value)
}

/// Writes a layout as JSON in the current format ([`SCHEMA_VERSION`](crate::SCHEMA_VERSION)).
///
/// # Errors
/// [`Error::Unreadable`] when the layout cannot be encoded.
pub fn store(layout: &Layout) -> Result<Vec<u8>> {
    serde_json::to_vec(layout).map_err(|source| Error::Unreadable(source.to_string()))
}

#[cfg(test)]
#[path = "model_tests.rs"]
mod tests;
