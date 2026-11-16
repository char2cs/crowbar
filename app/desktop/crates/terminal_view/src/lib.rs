//! GPUI view of the Crowbar terminal: grid painting, keyboard, mouse and IME input, selection, links,
//! find bar and scrollbar, over the `terminal` crate's emulator.

#![allow(
    clippy::too_many_arguments,
    reason = "the four-parameter limit of the manifest does not count `self`"
)]
#![allow(
    clippy::cast_precision_loss,
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "cell and pixel geometry stays far below f32 precision and the usize range"
)]

mod ansi_colors;
mod auto_scroll;
mod cached_row;
mod cursor_blink;
mod cursor_look;
mod drag_state;
mod focus_state;
mod frame;
mod frame_batch;
mod key_conversion;
mod line_cache;
mod page_direction;
mod paint_colors;
mod paint_cursor;
mod paint_highlights;
mod paint_scrollbar;
mod paint_text;
mod pointer_report;
mod search_bar;
mod search_bar_event;
mod search_session;
mod shaping;
mod shortcut;
mod terminal_font;
mod terminal_link;
mod terminal_peer;
mod terminal_settings;
mod terminal_view;
mod terminal_view_api;
mod terminal_view_event;
mod terminal_view_ime;
mod terminal_view_input;
mod terminal_view_mouse;
mod terminal_view_paint;
mod terminal_view_search;
mod terminal_view_session;
#[cfg(test)]
mod test_support;
mod theme_palette;
mod view_layout;

pub use terminal_link::TerminalLink;
pub use terminal_peer::TerminalPeer;
pub use terminal_settings::TerminalSettings;
pub use terminal_view::TerminalView;
pub use terminal_view_event::TerminalViewEvent;
