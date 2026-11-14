use std::path::PathBuf;

use tokio::sync::watch;

/// Where the local daemon's socket is, read afresh on every connect because a respawn or a late
/// publish can change it.
pub trait SocketLocator: Send + Sync {
    /// The socket path, `None` while none is published.
    fn socket_path(&self) -> Option<PathBuf>;
}

impl SocketLocator for PathBuf {
    fn socket_path(&self) -> Option<PathBuf> {
        Some(self.clone())
    }
}

impl SocketLocator for watch::Receiver<Option<PathBuf>> {
    fn socket_path(&self) -> Option<PathBuf> {
        self.borrow().clone()
    }
}

#[cfg(test)]
#[path = "socket_locator_tests.rs"]
mod tests;
