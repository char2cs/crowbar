use std::sync::Arc;

use crate::bounds::Bounds;
use crate::visibility::Visibility;
use crate::web_view_kind::WebViewKind;

/// Called with the text of every message the page posts through `window.ipc.postMessage`.
/// It runs on the platform thread and belongs to exactly one web view, so the handler's
/// closure is the sender's identity.
pub type IpcHandler = Arc<dyn Fn(String) + Send + Sync + 'static>;

/// What to create.
#[derive(Clone)]
pub struct WebViewConfig {
    /// The page to load.
    pub url: String,
    /// Initial placement.
    pub bounds: Bounds,
    /// Core or extension view.
    pub kind: WebViewKind,
    /// Initial visibility.
    pub visibility: Visibility,
    /// Script run in every page before its own scripts.
    pub initialization_script: Option<String>,
    /// Receives the page's ipc messages.
    pub on_ipc: IpcHandler,
}

impl std::fmt::Debug for WebViewConfig {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("WebViewConfig")
            .field("url", &self.url)
            .field("bounds", &self.bounds)
            .field("kind", &self.kind)
            .field("visibility", &self.visibility)
            .finish_non_exhaustive()
    }
}
