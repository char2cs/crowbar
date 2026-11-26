//! What pages post: IPC messages and envelopes, host events, the inbox gate, notices, frame kinds, page reports (hover, focus, pointer and drop), and browser page handlers.

mod browser_page;
mod frame_kind;
pub(crate) mod gate;
mod host_event;
mod ipc_message;
mod ipc_notice;
mod page_reports;

pub(crate) use browser_page::{browser_event, first_url, is_browser, page_handler, pick_handler};
pub(crate) use gate::{Gate, MAX_MESSAGE_BYTES, open_envelope};
pub use host_event::HostEvent;
pub use ipc_message::IpcMessage;
pub(crate) use ipc_notice::IpcNotice;
pub(crate) use page_reports::{
    drop_frame, drop_handler, focus_report, hover_report, pointer_report, press_handler,
};
