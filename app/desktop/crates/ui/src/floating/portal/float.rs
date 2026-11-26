//! Where a portal's content is drawn, and what it covers there.

/// Where a [`crate::Portal`] draws, on a platform whose native views cover what the window
/// paints itself. All but [`Float::Passive`] draw in the window and enter its
/// [`crate::Cover`], so the native views under them are hidden while they show.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub enum Float {
    /// A dialog: it covers the whole window.
    Modal,
    /// A menu or popover: it covers the cards it paints.
    Popover,
    /// Tooltips and toasts: a pop-up window over the owner that never takes the keyboard.
    Passive,
    /// The view a drag shows and its drop targets: it covers the whole window.
    Drag,
}
