//! The address bar strip above a browser pane.
//!
//! [`BrowserBar`] shows back, forward and reload buttons and an address field. It decides
//! nothing: it emits a [`BrowserBarEvent`] and the shell drives the web view.

mod browser_bar;
mod browser_bar_event;

pub use browser_bar::BrowserBar;
pub use browser_bar_event::BrowserBarEvent;
