use webview::{WebViewHandle, WebViewKind};

use crate::slot_id::SlotId;
use crate::slot_owner::SlotOwner;

/// A pooled web view and the pane it currently serves.
#[derive(Debug)]
pub(crate) struct Slot {
    pub(crate) id: SlotId,
    pub(crate) kind: WebViewKind,
    pub(crate) handle: WebViewHandle,
    pub(crate) owner: SlotOwner,
}
