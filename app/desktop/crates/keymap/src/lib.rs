//! Shortcut routing of the Crowbar shell.
//!
//! A key can be typed into the shell's own GPUI views or into a pane's web view. Both feed one
//! [`Keymap`]: the shell resolves a [`Keystroke`](chord::Keystroke) to a [`ShellCommand`] it
//! serves itself, and [`Keymap::forwarded`] lists the chords a pane must forward as `command.run`
//! (the page script prevents the default and posts the command id), so `mod+t` works whichever
//! side has focus.
//! Chords every pane keeps for itself (editor save, zoom) are never forwarded.

mod command_entry;
mod command_table;
mod keymap;
mod shell_command;
mod shell_keys;

pub use keymap::Keymap;
pub use shell_command::ShellCommand;
pub use shell_keys::{project_slot_of_command, project_slot_of_stroke};
