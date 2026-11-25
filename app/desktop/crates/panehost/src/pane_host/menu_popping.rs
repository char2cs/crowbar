use crate::pane_host::PaneHost;

impl PaneHost {
    /// The desktop's context menu, popped up on the web views' platform thread.
    #[must_use]
    pub fn native_menu(&self) -> webview::NativeMenu {
        self.factory.webviews().native_menu()
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
    use webview::{Call, MenuEdge, MenuEntry, MenuRequest, ParentWindow};

    use crate::test_support::fixture;

    #[test]
    fn the_menu_pops_up_on_the_web_views_platform() {
        let f = fixture();
        let request = MenuRequest {
            entries: vec![MenuEntry::Separator],
            x: 3.0,
            y: 5.0,
            edge: MenuEdge::Start,
        };
        let parent = ParentWindow::X11(7);
        f.host
            .native_menu()
            .popup(parent, request.clone(), Box::new(|_| ()))
            .unwrap();
        assert_eq!(f.mock.calls(), [Call::PopupMenu { parent, request }]);
    }
}
