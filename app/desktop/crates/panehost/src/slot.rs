mod bootstrap;
pub(crate) mod inbox;
mod owned_pane;
mod pane_host_config;
mod slot_factory;
mod slot_id;
mod slot_owner;
mod slot_pool;
mod slot_request;

pub use inbox::{HostEvent, IpcMessage};
pub(crate) use inbox::{
    InboxGate, IpcNotice, MAX_MESSAGE_BYTES, browser_event, drop_report, first_url, focus_report,
    is_browser, parse_hover,
};
pub use pane_host_config::PaneHostConfig;
pub(crate) use slot_factory::SlotFactory;
pub(crate) use slot_id::SlotId;
pub(crate) use slot_pool::SlotPool;
pub(crate) use slot_request::{BLANK_URL, SlotRequest};

use webview::{WebViewHandle, WebViewKind};

use crate::slot::slot_owner::SlotOwner;

#[derive(Debug)]
pub(crate) struct Slot {
    pub(crate) id: SlotId,
    pub(crate) kind: WebViewKind,
    pub(crate) handle: WebViewHandle,
    pub(crate) owner: SlotOwner,
}
