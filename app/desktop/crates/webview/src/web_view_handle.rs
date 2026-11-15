use std::rc::Rc;

use crate::bounds::Bounds;
use crate::error::Result;
use crate::platform::Platform;
use crate::visibility::Visibility;
use crate::web_view_id::WebViewId;

/// The JavaScript function a page defines to receive messages from [`WebViewHandle::post_message`].
pub const RECEIVER: &str = "__crowbarReceive";

/// A live web view. Dropping the handle destroys the view.
pub struct WebViewHandle {
    platform: Rc<dyn Platform>,
    id: WebViewId,
    destroyed: bool,
}

impl std::fmt::Debug for WebViewHandle {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("WebViewHandle")
            .field("id", &self.id)
            .finish_non_exhaustive()
    }
}

impl WebViewHandle {
    pub(crate) fn new(
        platform: Rc<dyn Platform>,
        id: WebViewId,
    ) -> Self {
        Self {
            platform,
            id,
            destroyed: false,
        }
    }

    /// The view's id, also carried by every [`ProtocolRequest`](crate::ProtocolRequest) it makes.
    #[must_use]
    pub fn id(&self) -> WebViewId {
        self.id
    }

    /// Moves and resizes the view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn set_bounds(
        &self,
        bounds: Bounds,
    ) -> Result<()> {
        self.platform.set_bounds(self.id, bounds)
    }

    /// Draws the view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn show(&self) -> Result<()> {
        self.platform.set_visibility(self.id, Visibility::Shown)
    }

    /// Stops drawing the view; the page stays loaded and is not told.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn hide(&self) -> Result<()> {
        self.platform.set_visibility(self.id, Visibility::Hidden)
    }

    /// Runs a script in the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn eval(
        &self,
        script: &str,
    ) -> Result<()> {
        self.platform.eval(self.id, script)
    }

    /// Loads another page into the view, keeping its native view and web process.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn navigate(
        &self,
        url: &str,
    ) -> Result<()> {
        self.platform.navigate(self.id, url)
    }

    /// Gives keyboard focus to the page.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn focus(&self) -> Result<()> {
        self.platform.focus(self.id)
    }

    /// Takes keyboard focus back to the parent window.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn focus_parent(&self) -> Result<()> {
        self.platform.focus_parent(self.id)
    }

    /// Delivers a text message to the page by calling its [`RECEIVER`] function.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn post_message(
        &self,
        text: &str,
    ) -> Result<()> {
        self.eval(&receive_script(text))
    }

    /// Destroys the view now and reports failures, which dropping would only log.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped.
    pub fn destroy(mut self) -> Result<()> {
        self.destroyed = true;
        self.platform.destroy(self.id)
    }
}

impl Drop for WebViewHandle {
    fn drop(&mut self) {
        if self.destroyed {
            return;
        }
        if let Err(error) = self.platform.destroy(self.id) {
            tracing::warn!(%error, view = %self.id, "destroying a web view on drop failed");
        }
    }
}

fn receive_script(text: &str) -> String {
    let literal = serde_json::to_string(text).unwrap_or_else(|_| String::from("\"\""));
    format!("window.{RECEIVER}&&window.{RECEIVER}({literal})")
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::bounds::Bounds;
    use crate::call::Call;
    use crate::mock_platform::MockPlatform;
    use crate::parent_window::ParentWindow;
    use crate::web_view_config::WebViewConfig;
    use crate::web_view_kind::WebViewKind;

    fn handle() -> (Rc<MockPlatform>, WebViewHandle) {
        let mock = Rc::new(MockPlatform::default());
        let bounds = Bounds {
            x: 0.0,
            y: 0.0,
            width: 1.0,
            height: 1.0,
        };
        let config = WebViewConfig {
            url: "x".into(),
            bounds,
            kind: WebViewKind::Core,
            visibility: Visibility::Shown,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        };
        mock.create(WebViewId(0), ParentWindow::X11(1), config)
            .unwrap();
        (Rc::clone(&mock), WebViewHandle::new(mock, WebViewId(0)))
    }

    #[test]
    fn show_and_hide_only_change_visibility() {
        let (mock, view) = handle();
        view.hide().unwrap();
        view.show().unwrap();
        let visibilities: Vec<Visibility> = mock
            .calls()
            .iter()
            .filter_map(|call| match call {
                Call::SetVisibility { visibility, .. } => Some(*visibility),
                _ => None,
            })
            .collect();
        assert_eq!(
            visibilities,
            [Visibility::Hidden, Visibility::Shown],
            "hide then show"
        );
        assert_eq!(
            mock.calls().len(),
            3,
            "create plus two visibility changes, nothing else"
        );
    }

    const CASES: &[(&str, &str, &str)] = &[
        (
            "quotes, newlines and a closing script tag",
            "a\"b\n</script>",
            r#"window.__crowbarReceive&&window.__crowbarReceive("a\"b\n</script>")"#,
        ),
        (
            "plain text",
            "hi",
            r#"window.__crowbarReceive&&window.__crowbarReceive("hi")"#,
        ),
    ];

    #[test]
    fn post_message_escapes_the_text_into_a_script_literal() {
        let wrong: Vec<&str> = CASES
            .iter()
            .filter(|(_, text, expected)| {
                let (mock, view) = handle();
                view.post_message(text).unwrap();
                let script = mock.calls().into_iter().find_map(|call| match call {
                    Call::Eval { script, .. } => Some(script),
                    _ => None,
                });
                script.as_deref() != Some(*expected)
            })
            .map(|(name, ..)| *name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
    }

    #[test]
    fn dropping_destroys_exactly_once() {
        let (mock, view) = handle();
        drop(view);
        assert_eq!(
            mock.live_views(),
            Vec::<WebViewId>::new(),
            "destroyed on drop"
        );
        let (mock, view) = handle();
        view.destroy().unwrap();
        let destroys = mock
            .calls()
            .iter()
            .filter(|c| matches!(c, Call::Destroy { .. }))
            .count();
        assert_eq!(destroys, 1, "explicit destroy is not repeated by drop");
    }
}
