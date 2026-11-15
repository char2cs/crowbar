use webview::{Bounds, ParentWindow, WebViewKind};

#[derive(Debug, Clone)]
pub(crate) struct SlotRequest {
    pub(crate) parent: ParentWindow,
    pub(crate) kind: WebViewKind,
    pub(crate) url: String,
    pub(crate) bounds: Bounds,
}
