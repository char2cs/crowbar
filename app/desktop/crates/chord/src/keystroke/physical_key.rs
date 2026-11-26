//! A key press as the keyboard saw it, before the layout and shift renamed the key.

use crate::keystroke::modifiers::Modifiers;

/// The pressed key's position, named as an unshifted US layout names it (`2` for the key that
/// types `@` with shift), with every modifier held, as the web's `KeyboardEvent.code` reports it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PhysicalKey {
    /// The key at this position on an unshifted US layout, for example `2` or `a`.
    pub code: String,
    /// The modifier keys held, shift included even when the toolkit folded it into the key.
    pub modifiers: Modifiers,
}
