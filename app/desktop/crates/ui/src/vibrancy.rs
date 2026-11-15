//! Whether the window lets the desktop show through its chrome.

/// The window's background capability, chosen by the host from the platform it runs on.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub enum Vibrancy {
    /// An opaque window: chrome washes are flattened to their opaque ink.
    #[default]
    None,
    /// A window with a blurred backdrop: chrome washes keep their alpha.
    Translucent,
}
