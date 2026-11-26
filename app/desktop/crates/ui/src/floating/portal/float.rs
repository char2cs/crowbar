//! Which float window of its owner a portal's content goes to.

/// The float window of its owner a [`crate::Portal`] paints in, on a platform whose native
/// views cover what the owner paints itself. Each kind is its own stock window over the owner.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum Float {
    /// A dialog: the window manager gives it the keyboard, and the owner takes no input while
    /// it is open.
    Modal,
    /// A menu or popover: it takes the keyboard, and the pointer on its cards.
    Popover,
    /// Tooltips and toasts: never the keyboard, the pointer only on cards that take it.
    Passive,
    /// The view a drag shows under the pointer: neither the pointer nor the keyboard.
    Drag,
}

impl Float {
    /// Whether its window takes the keyboard while it is open.
    pub const fn takes_keys(self) -> bool {
        matches!(self, Self::Modal | Self::Popover)
    }
}

#[cfg(test)]
mod tests {
    use super::Float;

    #[test]
    fn only_dialogs_and_menus_take_the_keyboard() {
        let takes =
            [Float::Modal, Float::Popover, Float::Passive, Float::Drag].map(Float::takes_keys);
        assert_eq!(takes, [true, true, false, false]);
    }
}
