//! What pages post: IPC messages and envelopes, host events, the inbox gate, notices, frame kinds, hover, focus, pointer and drop reports, and browser page handlers.

mod browser_page;
mod drop_message;
mod focus_message;
mod frame_kind;
pub(crate) mod gate;
mod host_event;
mod hover_message;
mod ipc_envelope;
mod ipc_message;
mod ipc_notice;
mod pointer_message;

pub(crate) use browser_page::{browser_event, first_url, is_browser, page_handler, pick_handler};
pub(crate) use drop_message::{drop_frame, drop_handler};
pub(crate) use focus_message::{focus_report, press_handler};
pub(crate) use gate::{Gate, MAX_MESSAGE_BYTES};
pub use host_event::HostEvent;
pub(crate) use hover_message::parse_hover;
pub(crate) use ipc_envelope::open_envelope;
pub use ipc_message::IpcMessage;
pub(crate) use ipc_notice::IpcNotice;
pub(crate) use pointer_message::pointer_report;
