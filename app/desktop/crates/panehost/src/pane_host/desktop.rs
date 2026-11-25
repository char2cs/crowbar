use crate::pane_host::PaneHost;

impl PaneHost {
    /// The desktop's own chooser, menu and dialog, shown on the web views' platform thread.
    #[must_use]
    pub fn desktop(&self) -> webview::Desktop {
        self.factory.desktop()
    }

    /// The native menus pages ask for. Await it on the main thread, pop each up over its pane
    /// and answer it.
    #[must_use]
    pub fn menu_asks(&self) -> async_channel::Receiver<crate::scheme::MenuAsk> {
        self.menus.asks()
    }
}

#[cfg(test)]
mod tests {
    use webview::{Call, ChoiceKind, FileChoice, ParentWindow};

    use crate::test_support::fixture;

    #[test]
    fn the_desktop_s_chooser_opens_on_the_web_views_platform() {
        let f = fixture();
        let choice = FileChoice {
            title: "Select Files",
            kind: ChoiceKind::Files,
            filter: None,
        };
        let parent = ParentWindow::X11(7);
        f.host
            .desktop()
            .choose(parent, choice, Box::new(|_| ()))
            .unwrap();
        assert_eq!(f.mock.calls(), [Call::ChooseFiles { parent, choice }]);
    }
}
