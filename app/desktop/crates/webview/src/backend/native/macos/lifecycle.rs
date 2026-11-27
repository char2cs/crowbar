//! Making and dropping a page on the `AppKit` backend.

use std::rc::Rc;
use std::sync::Arc;

use platform::{AppkitParent, PageClip};
use wry::{WebViewBuilder, WebViewBuilderExtDarwin};

use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::backend::native::macos::data_store::{BROWSER_STORE, store_identifier};
use crate::backend::native::macos::page::{Page, filling, frame_of};
use crate::backend::native::wry_builder::{common_attributes, register_scheme, to_rect};
use crate::backend::view::view_lifecycle::ViewLifecycle;
use crate::error::{Error, Result};
use crate::view::{Config, Id, Kind, ParentWindow};

fn appkit_address(parent: ParentWindow) -> Result<usize> {
    let ParentWindow::AppKit(address) = parent else {
        return Err(Error::UnsupportedParent(
            "a macOS host needs an AppKit window",
        ));
    };
    Ok(address)
}

// PLATFORM: macos the shell's pages show the window blur through them; a site keeps its ground.
const fn see_through(kind: &Kind) -> bool {
    !matches!(kind, Kind::Browser)
}

fn builder_for<'a>(
    platform: &'a AppkitPlatform,
    kind: &Kind,
) -> WebViewBuilder<'a> {
    let scheme =
        |builder| register_scheme(builder, &platform.scheme, Arc::clone(&platform.protocol));
    match kind {
        Kind::Core => scheme(WebViewBuilder::new()),
        Kind::Extension(extension) => {
            scheme(WebViewBuilder::new()).with_data_store_identifier(store_identifier(extension))
        },
        // PLATFORM: macos has no custom scheme here, so a browser page cannot reach the pane protocol; the fixed identifier gives it a data store apart from core and every extension.
        Kind::Browser => WebViewBuilder::new().with_data_store_identifier(BROWSER_STORE),
    }
}

impl ViewLifecycle for AppkitPlatform {
    fn create(
        &self,
        id: Id,
        parent: ParentWindow,
        config: Config,
    ) -> Result<()> {
        let address = appkit_address(parent)?;
        let view = AppkitParent::find(address, self.main_thread)
            .ok_or(Error::UnsupportedParent("no open window shows that NSView"))?;
        let builder = builder_for(self, &config.kind).with_transparent(see_through(&config.kind));
        let id_text = id.to_string();
        let builder = match config.background {
            Some(color) => builder.with_background_color(color.rgba()),
            None => builder,
        };
        let clip = PageClip::within(&view)?;
        clip.set_frame(frame_of(config.bounds));
        let web_view = common_attributes(builder, &id_text, &config)
            .with_bounds(to_rect(filling(config.bounds)))
            .build_as_child(&clip)?;
        let page = Page::new(web_view, clip);
        self.views.borrow_mut().insert(id, Rc::new(page));
        self.kinds.borrow_mut().insert(id, config.kind);
        self.last_parent.set(Some(address));
        Ok(())
    }

    fn destroy(
        &self,
        id: Id,
    ) -> Result<()> {
        self.kinds.borrow_mut().remove(&id);
        self.views
            .borrow_mut()
            .remove(&id)
            .map(drop)
            .ok_or(Error::UnknownWebView(id))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_shell_s_own_pages_are_see_through_and_a_browser_page_is_not() {
        assert!(see_through(&Kind::Core), "pane pages sit on the pane box");
        assert!(
            !see_through(&Kind::Browser),
            "a site expects its own ground"
        );
    }

    #[test]
    fn only_an_appkit_window_can_be_a_parent() {
        assert!(matches!(appkit_address(ParentWindow::AppKit(7)), Ok(7)));
        let refused = appkit_address(ParentWindow::X11(7));
        assert!(matches!(refused, Err(Error::UnsupportedParent(_))));
    }
}
