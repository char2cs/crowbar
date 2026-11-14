//! A key press as the capture rule sees it.

use crate::modifier::Modifier;

/// A key and the modifiers held with it; the key is a lowercase DOM-style name (`a`, `f5`, `escape`, `arrowleft`).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Pressed {
    pub(crate) key: String,
    pub(crate) held: Vec<Modifier>,
}

impl Pressed {
    /// `key` pressed while `held` were down.
    pub fn new(key: impl Into<String>, held: &[Modifier]) -> Self {
        Self {
            key: key.into().to_lowercase(),
            held: held.to_vec(),
        }
    }

    pub(crate) fn has(&self, modifier: Modifier) -> bool {
        self.held.contains(&modifier)
    }
}

#[cfg(test)]
#[path = "pressed_tests.rs"]
mod tests;
