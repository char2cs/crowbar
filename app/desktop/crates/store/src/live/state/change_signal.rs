//! How the shell learns that what the sidebar reads has changed.

use std::sync::Arc;

/// Called from any thread whenever the data changed; the shell turns it into a redraw.
pub type ChangeSignal = Arc<dyn Fn() + Send + Sync>;
