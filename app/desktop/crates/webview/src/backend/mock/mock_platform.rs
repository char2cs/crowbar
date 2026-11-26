use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::path::PathBuf;

use crate::backend::mock::call::Call;
use crate::backend::platform::Platform;
use crate::error::{Error, Result};
use crate::view::Id;
use crate::view::IpcHandler;

/// A backend that records calls instead of showing anything, for tests of code built on
/// [`Host`](crate::Host).
#[derive(Default)]
pub struct MockPlatform {
    pub(crate) calls: RefCell<Vec<Call>>,
    pub(crate) live: RefCell<HashMap<Id, IpcHandler>>,
    pub(crate) fail_next_create: Cell<bool>,
    pub(crate) chosen: RefCell<Vec<PathBuf>>,
    pub(crate) picked: RefCell<Option<Vec<usize>>>,
    pub(crate) pressed: Cell<Option<usize>>,
}

impl std::fmt::Debug for MockPlatform {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("MockPlatform")
            .field("calls", &self.calls)
            .finish_non_exhaustive()
    }
}

impl MockPlatform {
    /// Everything recorded so far, oldest first.
    #[must_use]
    pub fn calls(&self) -> Vec<Call> {
        self.calls.borrow().clone()
    }

    /// Ids of the views that are created and not yet destroyed.
    #[must_use]
    pub fn live_views(&self) -> Vec<Id> {
        let mut ids: Vec<Id> = self.live.borrow().keys().copied().collect();
        ids.sort();
        ids
    }

    /// Makes the next `create` fail with [`Error::HostStopped`].
    pub fn fail_next_create(&self) {
        self.fail_next_create.set(true);
    }

    /// Makes every later file chooser answer `paths`, as if the user chose them; until then
    /// each one is cancelled.
    pub fn choose_paths(
        &self,
        paths: Vec<PathBuf>,
    ) {
        self.chosen.replace(paths);
    }

    /// Makes every later context menu answer `path`, as if the user chose that entry; until
    /// then each one is dismissed.
    pub fn pick_entry(
        &self,
        path: Vec<usize>,
    ) {
        self.picked.replace(Some(path));
    }

    /// Makes every later message dialog answer `button`, as if the user pressed it; until then
    /// each one is dismissed.
    pub fn press_button(
        &self,
        button: usize,
    ) {
        self.pressed.set(Some(button));
    }

    /// Delivers `text` as if the page of `id` had posted it through ipc.
    ///
    /// # Errors
    /// [`Error::UnknownWebView`] when the view is not alive.
    pub fn simulate_ipc(
        &self,
        id: Id,
        text: &str,
    ) -> Result<()> {
        let handler = self
            .live
            .borrow()
            .get(&id)
            .cloned()
            .ok_or(Error::UnknownWebView(id))?;
        handler(text.to_owned());
        Ok(())
    }

    pub(crate) fn require_live(
        &self,
        id: Id,
    ) -> Result<()> {
        if self.live.borrow().contains_key(&id) {
            return Ok(());
        }
        Err(Error::UnknownWebView(id))
    }

    pub(crate) fn record(
        &self,
        call: Call,
    ) {
        self.calls.borrow_mut().push(call);
    }
}

impl Platform for MockPlatform {}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use super::*;
    use crate::backend::view::view_lifecycle::ViewLifecycle;
    use crate::test_support::view_fixtures::config;
    use crate::view::Config;
    use crate::view::Kind;
    use crate::view::ParentWindow;

    #[test]
    fn ipc_reaches_the_handler_of_the_right_view() {
        let mock = MockPlatform::default();
        let seen = Arc::new(Mutex::new(Vec::new()));
        let sink = Arc::clone(&seen);
        let listening = Config {
            on_ipc: Arc::new(move |text| sink.lock().unwrap().push(text)),
            ..config(Kind::Core)
        };
        mock.create(Id(1), ParentWindow::X11(1), listening).unwrap();
        mock.create(Id(2), ParentWindow::X11(1), config(Kind::Core))
            .unwrap();
        mock.simulate_ipc(Id(1), "hello").unwrap();
        mock.simulate_ipc(Id(2), "ignored").unwrap();
        assert_eq!(*seen.lock().unwrap(), ["hello"], "only view 1 saw it");
    }
}
