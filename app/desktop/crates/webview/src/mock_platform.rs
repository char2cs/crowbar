use std::cell::{Cell, RefCell};
use std::collections::HashMap;

use crate::bounds::Bounds;
use crate::call::Call;
use crate::error::{Error, Result};
use crate::parent_window::ParentWindow;
use crate::platform::Platform;
use crate::visibility::Visibility;
use crate::web_view_config::{IpcHandler, WebViewConfig};
use crate::web_view_id::WebViewId;

/// A backend that records calls instead of showing anything, for tests of code built on
/// [`WebViewHost`](crate::WebViewHost).
#[derive(Default)]
pub struct MockPlatform {
    calls: RefCell<Vec<Call>>,
    live: RefCell<HashMap<WebViewId, IpcHandler>>,
    fail_next_create: Cell<bool>,
}

impl std::fmt::Debug for MockPlatform {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
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
    pub fn live_views(&self) -> Vec<WebViewId> {
        let mut ids: Vec<WebViewId> = self.live.borrow().keys().copied().collect();
        ids.sort();
        ids
    }

    /// Makes the next `create` fail with [`Error::HostStopped`].
    pub fn fail_next_create(&self) {
        self.fail_next_create.set(true);
    }

    /// Delivers `text` as if the page of `id` had posted it through ipc.
    ///
    /// # Errors
    /// [`Error::UnknownWebView`] when the view is not alive.
    pub fn simulate_ipc(&self, id: WebViewId, text: &str) -> Result<()> {
        let handler = self
            .live
            .borrow()
            .get(&id)
            .cloned()
            .ok_or(Error::UnknownWebView(id))?;
        handler(text.to_owned());
        Ok(())
    }

    fn require_live(&self, id: WebViewId) -> Result<()> {
        if self.live.borrow().contains_key(&id) {
            Ok(())
        } else {
            Err(Error::UnknownWebView(id))
        }
    }

    fn record(&self, call: Call) {
        self.calls.borrow_mut().push(call);
    }
}

impl Platform for MockPlatform {
    fn create(&self, id: WebViewId, parent: ParentWindow, config: WebViewConfig) -> Result<()> {
        if self.fail_next_create.replace(false) {
            return Err(Error::HostStopped);
        }
        self.record(Call::Create {
            id,
            parent,
            url: config.url,
            kind: config.kind,
            bounds: config.bounds,
            visibility: config.visibility,
        });
        self.live.borrow_mut().insert(id, config.on_ipc);
        Ok(())
    }

    fn set_bounds(&self, id: WebViewId, bounds: Bounds) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetBounds { id, bounds });
        Ok(())
    }

    fn set_visibility(&self, id: WebViewId, visibility: Visibility) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::SetVisibility { id, visibility });
        Ok(())
    }

    fn eval(&self, id: WebViewId, script: &str) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Eval {
            id,
            script: script.to_owned(),
        });
        Ok(())
    }

    fn navigate(&self, id: WebViewId, url: &str) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Navigate {
            id,
            url: url.to_owned(),
        });
        Ok(())
    }

    fn focus(&self, id: WebViewId) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::Focus { id });
        Ok(())
    }

    fn focus_parent(&self, id: WebViewId) -> Result<()> {
        self.require_live(id)?;
        self.record(Call::FocusParent { id });
        Ok(())
    }

    fn destroy(&self, id: WebViewId) -> Result<()> {
        self.live
            .borrow_mut()
            .remove(&id)
            .ok_or(Error::UnknownWebView(id))?;
        self.record(Call::Destroy { id });
        Ok(())
    }
}

#[cfg(test)]
#[path = "mock_platform_tests.rs"]
mod tests;
