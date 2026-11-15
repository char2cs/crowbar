use webview::{WebViewHandle, WebViewKind};

use crate::slot_id::SlotId;
use crate::slot_owner::SlotOwner;

#[derive(Debug)]
pub(crate) struct Slot {
    pub(crate) id: SlotId,
    pub(crate) kind: WebViewKind,
    pub(crate) handle: WebViewHandle,
    pub(crate) owner: SlotOwner,
}
