//! What a terminal view tells its host.

use gpui::{Pixels, Point};

/// Events the host acts on; the view never opens anything itself.
#[derive(Debug, Clone, PartialEq)]
pub enum Event {
    /// The program set a (sanitised, non-empty) title; an empty string clears it.
    TitleChanged(String),
    /// The bell rang.
    Bell,
    /// The program replaced the clipboard (OSC 52) with this many bytes; the host may tell the user.
    ClipboardWritten(usize),
    /// The session's process ended with this code.
    Exited(i32),
    /// A web link was activated with the modifier held; the host decides how to open it.
    OpenUrl {
        /// The address.
        url: String,
        /// Pointer position in window coordinates, for a menu.
        position: Point<Pixels>,
    },
    /// A file link was activated; `path` is absolute.
    OpenFile {
        /// Absolute path.
        path: String,
        /// One-based line to reveal.
        line: Option<u32>,
    },
    /// A file link could not be resolved to an absolute path.
    UnresolvedFile(String),
    /// Zoom changed the font size; the host stores it in the settings.
    FontSizeChanged(f32),
}
