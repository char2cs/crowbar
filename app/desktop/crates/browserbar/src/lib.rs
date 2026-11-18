//! The address bar strip above a browser pane.
//!
//! [`BrowserBar`] shows back, forward and reload buttons and an address field. It decides
//! nothing: it emits a [`Event`] and the shell drives the web view.

mod browser_bar;
mod event;

pub use browser_bar::BrowserBar;
pub use event::Event;
