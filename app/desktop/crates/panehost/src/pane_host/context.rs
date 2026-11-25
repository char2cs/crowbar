//! What panes are told: broadcasts, settings, paired hosts and the background.

mod background;
mod breakpoints;
mod broadcast;
mod chat_zoom;
mod paired_hosts;
mod settings;

pub(crate) use settings::setting_event;
