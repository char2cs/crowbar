use std::path::PathBuf;
use std::sync::{Arc, Mutex, MutexGuard, PoisonError};

use supervisor::{DaemonPaths, Supervisor};
use tokio::sync::{MutexGuard as TurnGuard, watch};

use crate::launched::Launched;

#[derive(Debug)]
pub(crate) struct LinkState {
    launched: Mutex<Launched>,
    socket: watch::Sender<Option<PathBuf>>,
    turn: tokio::sync::Mutex<()>,
}

pub(crate) fn linked() -> (LinkState, watch::Receiver<Option<PathBuf>>) {
    let (socket, locator) = watch::channel(None);
    (LinkState::new(socket), locator)
}

impl LinkState {
    fn new(socket: watch::Sender<Option<PathBuf>>) -> Self {
        Self {
            launched: Mutex::default(),
            socket,
            turn: tokio::sync::Mutex::new(()),
        }
    }

    fn lock(&self) -> MutexGuard<'_, Launched> {
        self.launched.lock().unwrap_or_else(PoisonError::into_inner)
    }

    pub(crate) fn adopt(
        &self,
        supervisor: Arc<Supervisor>,
        paths: DaemonPaths,
    ) {
        let mut launched = self.lock();
        launched.supervisor = Some(supervisor);
        launched.log = Some(paths.log);
        self.socket.send_replace(Some(paths.socket));
    }

    pub(crate) fn supervisor(&self) -> Option<Arc<Supervisor>> {
        self.lock().supervisor.clone()
    }

    pub(crate) fn take_supervisor(&self) -> Option<Arc<Supervisor>> {
        self.lock().supervisor.take()
    }

    pub(crate) fn log(&self) -> Option<PathBuf> {
        self.lock().log.clone()
    }

    pub(crate) fn socket(&self) -> Option<PathBuf> {
        self.socket.borrow().clone()
    }

    pub(crate) async fn turn(&self) -> TurnGuard<'_, ()> {
        self.turn.lock().await
    }
}

#[cfg(test)]
mod tests {

    use super::*;

    fn runtime() -> tokio::runtime::Runtime {
        tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap()
    }

    #[test]
    fn a_fresh_state_has_nothing_launched() {
        let (state, locator) = linked();
        assert!(state.supervisor().is_none(), "no supervisor");
        assert!(state.take_supervisor().is_none(), "nothing to take");
        assert_eq!(
            (state.log(), state.socket(), locator.borrow().clone()),
            (None, None, None),
            "no paths"
        );
    }

    #[test]
    fn adopting_a_launch_publishes_its_paths_and_hands_the_supervisor_out_once() {
        let runtime = runtime();
        let (state, locator) = linked();
        let paths = DaemonPaths {
            socket: PathBuf::from("/run/daemon.sock"),
            log: PathBuf::from("/run/daemon.log"),
        };
        let config = supervisor::Config::new(
            PathBuf::from("/nonexistent/daemon"),
            paths.clone(),
            None,
            "0.0.0",
        );
        let supervisor = runtime.block_on(async { Arc::new(Supervisor::start(config)) });
        state.adopt(supervisor, paths.clone());
        assert_eq!(state.log(), Some(paths.log), "log");
        assert_eq!(state.socket(), Some(paths.socket.clone()), "socket");
        assert_eq!(*locator.borrow(), Some(paths.socket), "locator follows");
        assert!(state.supervisor().is_some(), "kept for restarts");
        let taken = state.take_supervisor().unwrap();
        assert!(state.take_supervisor().is_none(), "handed out once");
        drop(state);
        let owned = Arc::try_unwrap(taken).unwrap_or_else(|_| panic!("sole owner"));
        runtime.block_on(owned.stop());
    }
}
