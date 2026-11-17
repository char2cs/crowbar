use std::cell::{Cell, RefCell};
use std::collections::HashMap;

use crate::call::Call;
use crate::error::{Error, Result};
use crate::platform::Platform;
use crate::web_view_config::IpcHandler;
use crate::web_view_id::WebViewId;

/// A backend that records calls instead of showing anything, for tests of code built on
/// [`WebViewHost`](crate::WebViewHost).
#[derive(Default)]
pub struct MockPlatform {
    pub(crate) calls: RefCell<Vec<Call>>,
    pub(crate) live: RefCell<HashMap<WebViewId, IpcHandler>>,
    pub(crate) fail_next_create: Cell<bool>,
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
    pub fn simulate_ipc(
        &self,
        id: WebViewId,
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
        id: WebViewId,
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
    use crate::bounds::Bounds;
    use crate::page_control::PageControl;
    use crate::parent_window::ParentWindow;
    use crate::view_layout::ViewLayout;
    use crate::view_lifecycle::ViewLifecycle;
    use crate::visibility::Visibility;
    use crate::web_view_config::WebViewConfig;
    use crate::web_view_kind::WebViewKind;

    fn rect(
        x: f64,
        y: f64,
        width: f64,
        height: f64,
    ) -> Bounds {
        Bounds {
            x,
            y,
            width,
            height,
        }
    }

    fn config(on_ipc: IpcHandler) -> WebViewConfig {
        WebViewConfig {
            url: "crowbar://local/".into(),
            bounds: rect(0.0, 0.0, 10.0, 10.0),
            kind: WebViewKind::Core,
            visibility: Visibility::Shown,
            background: None,
            initialization_script: None,
            on_ipc,
            on_page: None,
        }
    }

    #[test]
    fn records_the_life_of_a_view() {
        let mock = MockPlatform::default();
        let id = WebViewId(1);
        mock.create(id, ParentWindow::X11(9), config(Arc::new(|_| {})))
            .unwrap();
        mock.set_visibility(id, Visibility::Hidden).unwrap();
        mock.destroy(id).unwrap();
        let kinds: Vec<&str> = mock
            .calls()
            .iter()
            .map(|call| match call {
                Call::Create { .. } => "create",
                Call::SetVisibility { .. } => "visibility",
                Call::Destroy { .. } => "destroy",
                _ => "other",
            })
            .collect();
        assert_eq!(kinds, ["create", "visibility", "destroy"], "order");
        assert!(mock.live_views().is_empty(), "nothing alive");
    }

    type Operation = fn(&MockPlatform, WebViewId) -> Result<()>;

    const CASES: &[(&str, Operation)] = &[
        ("eval", |mock, id| mock.eval(id, "1")),
        ("bounds", |mock, id| {
            mock.set_bounds(id, rect(0.0, 0.0, 1.0, 1.0))
        }),
        ("destroy", |mock, id| mock.destroy(id)),
        ("ipc", |mock, id| mock.simulate_ipc(id, "x")),
    ];

    #[test]
    fn unknown_views_are_errors() {
        let mock = MockPlatform::default();
        for (name, operation) in CASES {
            assert!(
                matches!(
                    operation(&mock, WebViewId(5)),
                    Err(Error::UnknownWebView(_))
                ),
                "{name}"
            );
        }
    }

    #[test]
    fn ipc_reaches_the_handler_of_the_right_view() {
        let mock = MockPlatform::default();
        let seen = Arc::new(Mutex::new(Vec::new()));
        let sink = Arc::clone(&seen);
        mock.create(
            WebViewId(1),
            ParentWindow::X11(1),
            config(Arc::new(move |text| sink.lock().unwrap().push(text))),
        )
        .unwrap();
        mock.create(WebViewId(2), ParentWindow::X11(1), config(Arc::new(|_| {})))
            .unwrap();
        mock.simulate_ipc(WebViewId(1), "hello").unwrap();
        mock.simulate_ipc(WebViewId(2), "ignored").unwrap();
        assert_eq!(
            *seen.lock().unwrap(),
            ["hello"],
            "only view 1 saw its message"
        );
    }

    #[test]
    fn a_create_can_be_made_to_fail_once() {
        let mock = MockPlatform::default();
        mock.fail_next_create();
        assert!(
            mock.create(WebViewId(1), ParentWindow::X11(1), config(Arc::new(|_| {})))
                .is_err(),
            "first fails"
        );
        assert!(
            mock.create(WebViewId(1), ParentWindow::X11(1), config(Arc::new(|_| {})))
                .is_ok(),
            "second works"
        );
    }
}
