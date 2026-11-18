use webview::{Bounds, ParentWindow};

pub(crate) const BLANK_URL: &str = "about:blank";

#[derive(Debug, Clone)]
pub(crate) struct SlotRequest {
    pub(crate) parent: ParentWindow,
    pub(crate) kind: webview::Kind,
    pub(crate) bounds: Bounds,
}
