use crate::pane_host::PaneHost;

impl PaneHost {
    /// The desktop's file chooser, opened on the web views' platform thread.
    #[must_use]
    pub fn file_chooser(&self) -> webview::FileChooser {
        self.factory.file_chooser()
    }
}

#[cfg(test)]
mod tests {
    use webview::{Call, ChoiceKind, FileChoice, ParentWindow};

    use crate::test_support::fixture;

    #[test]
    fn the_chooser_opens_on_the_web_views_platform() {
        let f = fixture();
        let choice = FileChoice {
            title: "Select Files",
            kind: ChoiceKind::Files,
            filter: None,
        };
        let parent = ParentWindow::X11(7);
        f.host
            .file_chooser()
            .choose(parent, choice, Box::new(|_| ()))
            .unwrap();
        assert_eq!(f.mock.calls(), [Call::ChooseFiles { parent, choice }]);
    }
}
