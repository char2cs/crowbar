mod bootstrap;
mod config;
mod factory;
mod id;
pub(crate) mod inbox;
mod owned_pane;
mod owner;
mod pool;
mod request;

pub use config::Config;
pub(crate) use factory::Factory;
pub(crate) use id::Id;
pub(crate) use inbox::{
    Gate, IpcNotice, MAX_MESSAGE_BYTES, browser_event, drop_report, first_url, focus_report,
    is_browser, parse_hover,
};
pub use inbox::{HostEvent, IpcMessage};
pub(crate) use pool::Pool;
pub(crate) use request::{BLANK_URL, Request};

use crate::slot::owner::Owner;

#[derive(Debug)]
pub(crate) struct Slot {
    pub(crate) id: Id,
    pub(crate) kind: webview::Kind,
    pub(crate) handle: webview::Handle,
    pub(crate) owner: Owner,
}
