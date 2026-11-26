//! The request pump and what the shell does with each event.

pub(crate) mod backend;
mod child_events;
mod data_events;
mod evictions;
pub(crate) mod first_run;
mod notices;
mod outcomes;
mod pane_toasts;
mod pump;
mod review_viewed;
mod window_keyboard;
