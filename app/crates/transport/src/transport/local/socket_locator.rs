use std::path::PathBuf;

use tokio::sync::watch;

pub trait SocketLocator: Send + Sync {
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
mod tests {
    use super::*;

    #[test]
    fn a_path_locates_itself() {
        let path = PathBuf::from("/tmp/crowbar.sock");
        assert_eq!(path.socket_path(), Some(path));
    }

    #[test]
    fn a_watched_path_follows_the_supervisor_and_is_absent_until_published() {
        let (publish, locator) = watch::channel(None);
        assert_eq!(locator.socket_path(), None);
        publish.send_replace(Some(PathBuf::from("/tmp/a.sock")));
        assert_eq!(locator.socket_path(), Some(PathBuf::from("/tmp/a.sock")));
        publish.send_replace(Some(PathBuf::from("/tmp/b.sock")));
        assert_eq!(locator.socket_path(), Some(PathBuf::from("/tmp/b.sock")));
    }
}
