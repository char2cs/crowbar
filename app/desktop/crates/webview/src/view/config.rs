use std::sync::Arc;

use crate::view::bounds::Bounds;
use crate::view::color::Color;
use crate::view::kind::Kind;
use crate::view::page_event::PageHandler;
use crate::view::visibility::Visibility;

/// Called with the text of every message the page posts through `window.ipc.postMessage`.
/// It runs on the platform thread and belongs to exactly one web view, so the handler's
/// closure is the sender's identity.
pub type IpcHandler = Arc<dyn Fn(String) + Send + Sync + 'static>;

/// What to create.
#[derive(Clone)]
pub struct Config {
    /// The page to load.
    pub url: String,
    /// Initial placement.
    pub bounds: Bounds,
    /// Core or extension view.
    pub kind: Kind,
    /// Initial visibility.
    pub visibility: Visibility,
    /// What the view shows before its page paints; `None` leaves the engine default.
    pub background: Option<Color>,
    /// Script run in every page before its own scripts.
    pub initialization_script: Option<String>,
    /// Receives the page's ipc messages.
    pub on_ipc: IpcHandler,
    /// Receives the engine's page events; `None` for views whose navigation the shell does not watch.
    pub on_page: Option<PageHandler>,
}

impl std::fmt::Debug for Config {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Config")
            .field("url", &self.url)
            .field("bounds", &self.bounds)
            .field("kind", &self.kind)
            .field("visibility", &self.visibility)
            .finish_non_exhaustive()
    }
}
