/// Where a typed character sits on the keyboard, named by the key at that place on an unshifted
/// US layout, as the browser's `KeyboardEvent.code` names it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct UsKey {
    /// The key at that place on an unshifted US layout, for example `2` for a typed `@`.
    pub key: char,
    /// Whether the character takes shift on the active layout.
    pub shifted: bool,
}
