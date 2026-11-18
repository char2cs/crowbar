//! The one chord model of the Crowbar shell.
//!
//! A [`Chord`] is the canonical `mod+shift+alt+key` text the settings page stores, the shell
//! keymap matches and a pane forwards; [`Primary`] says which key plays `mod` and gives the label.
//! A [`Keystroke`] is one key press as the windowing layer reports it, with its held [`Modifiers`].

mod chord;
mod keystroke;
mod primary;

pub use chord::Chord;
pub use keystroke::{Keystroke, Modifier, Modifiers};
pub use primary::Primary;
