use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;
use std::sync::Arc;

use objc2_foundation::MainThreadMarker;
use platform::AppkitParent;
use wry::{WebView, WebViewBuilder, WebViewBuilderExtDarwin};

use crate::backend::host_config::HostConfig;
use crate::backend::native::macos::data_store::{BROWSER_STORE, store_identifier};
use crate::backend::native::wry_builder::{common_attributes, register_scheme, to_rect};
use crate::backend::page_control::PageControl;
use crate::backend::platform::Platform;
use crate::backend::view_capture::ViewCapture;
use crate::backend::view_focus::ViewFocus;
use crate::backend::view_layout::ViewLayout;
use crate::backend::view_lifecycle::ViewLifecycle;
use crate::error::{Error, Result};
use crate::scheme::ProtocolHandler;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Config;
use crate::view::Id;
use crate::view::Kind;
use crate::view::ParentWindow;
use crate::view::Visibility;

pub(crate) struct AppkitPlatform {
    scheme: String,
    protocol: Arc<dyn ProtocolHandler>,
    pub(crate) views: RefCell<HashMap<Id, Rc<WebView>>>,
    pub(crate) kinds: RefCell<HashMap<Id, Kind>>,
    pub(crate) last_parent: Cell<Option<usize>>,
    pub(crate) main_thread: MainThreadMarker,
}

impl AppkitPlatform {
    pub(crate) fn start(config: HostConfig) -> Result<Self> {
        let main_thread = MainThreadMarker::new().ok_or(Error::NotMainThread)?;
        Ok(Self {
            scheme: config.scheme,
            protocol: config.protocol,
            views: RefCell::new(HashMap::new()),
            kinds: RefCell::new(HashMap::new()),
            last_parent: Cell::new(None),
            main_thread,
        })
    }

    fn with_view<R>(
        &self,
        id: Id,
        run: impl FnOnce(&WebView) -> wry::Result<R>,
    ) -> Result<R> {
        let view = self
            .views
            .borrow()
            .get(&id)
            .cloned()
            .ok_or(Error::UnknownWebView(id))?;
        Ok(run(&view)?)
    }
}

impl AppkitPlatform {
    fn builder_for(
        &self,
        kind: &Kind,
    ) -> WebViewBuilder<'_> {
        let scheme = |builder| register_scheme(builder, &self.scheme, Arc::clone(&self.protocol));
        match kind {
            Kind::Core => scheme(WebViewBuilder::new()),
            Kind::Extension(extension) => scheme(WebViewBuilder::new())
                .with_data_store_identifier(store_identifier(extension)),
            // PLATFORM: macos has no custom scheme here, so a browser page cannot reach the pane protocol; the fixed identifier gives it a data store apart from core and every extension.
            Kind::Browser => WebViewBuilder::new().with_data_store_identifier(BROWSER_STORE),
        }
    }
}

impl ViewLifecycle for AppkitPlatform {
    fn create(
        &self,
        id: Id,
        parent: ParentWindow,
        config: Config,
    ) -> Result<()> {
        let ParentWindow::AppKit(address) = parent else {
            return Err(Error::UnsupportedParent(
                "a macOS host needs an AppKit window",
            ));
        };
        let view = AppkitParent::find(address, self.main_thread)
            .ok_or(Error::UnsupportedParent("no open window shows that NSView"))?;
        let builder = self.builder_for(&config.kind);
        let id_text = id.to_string();
        let builder = match config.background {
            Some(color) => builder.with_background_color(color.rgba()),
            None => builder,
        };
        let web_view = common_attributes(builder, &id_text, &config).build_as_child(&view)?;
        self.views.borrow_mut().insert(id, Rc::new(web_view));
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

impl ViewLayout for AppkitPlatform {
    fn set_bounds(
        &self,
        id: Id,
        bounds: Bounds,
    ) -> Result<()> {
        self.with_view(id, |view| view.set_bounds(to_rect(bounds)))
    }

    fn set_visibility(
        &self,
        id: Id,
        visibility: Visibility,
    ) -> Result<()> {
        self.with_view(id, |view| view.set_visible(visibility == Visibility::Shown))
    }

    fn set_background(
        &self,
        id: Id,
        color: Color,
    ) -> Result<()> {
        self.with_view(id, |view| view.set_background_color(color.rgba()))
    }
}

impl PageControl for AppkitPlatform {
    fn eval(
        &self,
        id: Id,
        script: &str,
    ) -> Result<()> {
        self.with_view(id, |view| view.evaluate_script(script))
    }

    fn navigate(
        &self,
        id: Id,
        url: &str,
    ) -> Result<()> {
        self.with_view(id, |view| view.load_url(url))
    }
}

impl Platform for AppkitPlatform {}

impl ViewCapture for AppkitPlatform {}

impl ViewFocus for AppkitPlatform {
    fn focus(
        &self,
        id: Id,
    ) -> Result<()> {
        self.with_view(id, WebView::focus)
    }

    fn focus_parent(
        &self,
        id: Id,
    ) -> Result<()> {
        self.with_view(id, WebView::focus_parent)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::scheme::ProtocolRequest;
    use crate::scheme::ProtocolResponder;

    struct Silent;

    impl ProtocolHandler for Silent {
        fn handle(
            &self,
            _request: ProtocolRequest,
            _respond: ProtocolResponder,
        ) {
        }
    }

    #[test]
    fn a_backend_cannot_start_off_the_main_thread() {
        let config = HostConfig {
            scheme: "crowbar".into(),
            protocol: Arc::new(Silent),
            data_dir: None,
        };
        let started = std::thread::spawn(move || AppkitPlatform::start(config).map(drop))
            .join()
            .unwrap();
        assert!(matches!(started, Err(Error::NotMainThread)), "refused");
    }
}
