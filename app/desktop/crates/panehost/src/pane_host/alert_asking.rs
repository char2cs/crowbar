use crate::pane_host::PaneHost;

impl PaneHost {
    /// The desktop's message dialog, asked on the web views' platform thread.
    #[must_use]
    pub fn native_alert(&self) -> webview::NativeAlert {
        self.factory.webviews().native_alert()
    }
}

#[cfg(test)]
mod tests {
    use webview::{AlertRequest, Call, ParentWindow};

    use crate::test_support::fixture;

    #[test]
    fn the_dialog_is_asked_on_the_web_views_platform() {
        let f = fixture();
        let request = AlertRequest {
            title: None,
            message: "Reload?".into(),
            buttons: vec![],
        };
        let parent = ParentWindow::X11(7);
        f.host
            .native_alert()
            .ask(parent, request.clone(), Box::new(|_| ()))
            .unwrap();
        assert_eq!(f.mock.calls(), [Call::AskAlert { parent, request }]);
    }
}
