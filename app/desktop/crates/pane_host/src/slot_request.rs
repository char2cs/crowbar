use webview::{Bounds, ParentWindow, WebViewKind};

pub(crate) const BLANK_URL: &str = "about:blank";

#[derive(Debug, Clone)]
pub(crate) struct SlotRequest {
    pub(crate) parent: ParentWindow,
    pub(crate) kind: WebViewKind,
    pub(crate) bounds: Bounds,
}
