//! Shortcut routing of the Crowbar shell.
//!
//! A key can be typed into the shell's own GPUI views or into a pane's web view. Both feed one
//! [`Keymap`]: the shell resolves a [`Keystroke`](chord::Keystroke) to a [`ShellCommand`] it
//! serves itself, and [`Keymap::forwarded`] lists the chords a pane must forward as `command.run`
//! (the page script prevents the default and posts the command id), so `mod+t` works whichever
//! side has focus.
//! Chat zoom runs beside the focused view instead of replacing it ([`Keymap::alongside`]):
//! develop zooms the chat from anywhere, and a focused terminal still zooms its font.
//! Chords every pane keeps for itself (editor save) are never forwarded. A
//! [`ContextualCommand`] is forwarded only while its surface shows, and yields to a page that
//! handled the key.

mod contextual_command;
mod keymap;
mod project_slot;
mod shell_command;

pub use contextual_command::ContextualCommand;
pub use keymap::Keymap;
pub use project_slot::{project_slot_of_command, project_slot_of_stroke};
pub use shell_command::ShellCommand;
