use crate::pane_host::PaneHost;

impl PaneHost {
    /// The desktop's context menu, popped up on the web views' platform thread.
    #[must_use]
    pub fn native_menu(&self) -> webview::NativeMenu {
        self.factory.native_menu()
    }
}

#[cfg(test)]
mod tests {
    use webview::{Call, MenuEntry, MenuRequest, ParentWindow};

    use crate::test_support::fixture;

    #[test]
    fn the_menu_pops_up_on_the_web_views_platform() {
        let f = fixture();
        let request = MenuRequest {
            entries: vec![MenuEntry::Separator],
            x: 3.0,
            y: 5.0,
        };
        let parent = ParentWindow::X11(7);
        f.host
            .native_menu()
            .popup(parent, request.clone(), Box::new(|_| ()))
            .unwrap();
        assert_eq!(f.mock.calls(), [Call::PopupMenu { parent, request }]);
    }
}
