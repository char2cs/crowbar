//! A key press in toolkit-neutral form.

use crate::modifiers::Modifiers;

/// A key press: the key's name (`"a"`, `"enter"`, `"f5"`, `"pageup"`), the text it types, and the modifiers.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct KeyInput {
    /// Lower-case key name.
    pub key: String,
    /// Text the key produces with the modifiers applied, when it produces any.
    pub text: Option<String>,
    /// Modifiers held.
    pub modifiers: Modifiers,
}
