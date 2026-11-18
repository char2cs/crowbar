//! GPUI view of the Crowbar terminal: grid painting, keyboard, mouse and IME input, selection, links,
//! find bar and scrollbar, over the `terminal` crate's emulator.

mod daemon;
mod search_bar;
mod settings;
mod terminal_view;
#[cfg(test)]
mod test_support;

pub use daemon::{Link, Peer};
pub use settings::Settings;
pub use terminal_view::{Event, TerminalView};
