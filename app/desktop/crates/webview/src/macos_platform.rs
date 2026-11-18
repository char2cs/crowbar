use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;
use std::sync::Arc;

use objc2_foundation::MainThreadMarker;
use platform::AppkitParent;
use wry::{WebView, WebViewBuilder, WebViewBuilderExtDarwin};

use crate::bounds::Bounds;
use crate::builder_support::{common_attributes, register_scheme, to_rect};
use crate::color::Color;
use crate::error::{Error, Result};
use crate::host_config::HostConfig;
use crate::macos_data_store::{BROWSER_STORE, store_identifier};
use crate::page_control::PageControl;
use crate::parent_window::ParentWindow;
use crate::platform::Platform;
use crate::protocol_handler::ProtocolHandler;
use crate::view_capture::ViewCapture;
use crate::view_focus::ViewFocus;
use crate::view_layout::ViewLayout;
use crate::view_lifecycle::ViewLifecycle;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;
use crate::web_view_kind::WebViewKind;

pub(crate) struct MacosPlatform {
    scheme: String,
    protocol: Arc<dyn ProtocolHandler>,
    pub(crate) views: RefCell<HashMap<WebViewId, Rc<WebView>>>,
    pub(crate) kinds: RefCell<HashMap<WebViewId, WebViewKind>>,
    pub(crate) last_parent: Cell<Option<usize>>,
    pub(crate) main_thread: MainThreadMarker,
}

impl MacosPlatform {
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
        id: WebViewId,
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

impl MacosPlatform {
    fn builder_for(
        &self,
        kind: &WebViewKind,
    ) -> WebViewBuilder<'_> {
        let scheme = |builder| register_scheme(builder, &self.scheme, Arc::clone(&self.protocol));
        match kind {
            WebViewKind::Core => scheme(WebViewBuilder::new()),
            WebViewKind::Extension(extension) => scheme(WebViewBuilder::new())
                .with_data_store_identifier(store_identifier(extension)),
            // PLATFORM: macos has no custom scheme here, so a browser page cannot reach the pane protocol; the fixed identifier gives it a data store apart from core and every extension.
            WebViewKind::Browser => WebViewBuilder::new().with_data_store_identifier(BROWSER_STORE),
        }
    }
}

impl ViewLifecycle for MacosPlatform {
    fn create(
        &self,
        id: WebViewId,
        parent: ParentWindow,
        config: WebViewConfig,
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
        let web_view = common_attributes(builder, &id_text, &config).build_as_child(&view)?;
        self.views.borrow_mut().insert(id, Rc::new(web_view));
        self.kinds.borrow_mut().insert(id, config.kind);
        self.last_parent.set(Some(address));
        Ok(())
    }

    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.kinds.borrow_mut().remove(&id);
        self.views
            .borrow_mut()
            .remove(&id)
            .map(drop)
            .ok_or(Error::UnknownWebView(id))
    }
}

impl ViewLayout for MacosPlatform {
    fn set_bounds(
        &self,
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()> {
        self.with_view(id, |view| view.set_bounds(to_rect(bounds)))
    }

    fn set_visibility(
        &self,
        id: WebViewId,
        visibility: Visibility,
    ) -> Result<()> {
        self.with_view(id, |view| view.set_visible(visibility == Visibility::Shown))
    }

    fn set_background(
        &self,
        id: WebViewId,
        color: Color,
    ) -> Result<()> {
        self.with_view(id, |view| view.set_background_color(color.rgba()))
    }
}

impl PageControl for MacosPlatform {
    fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()> {
        self.with_view(id, |view| view.evaluate_script(script))
    }

    fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()> {
        self.with_view(id, |view| view.load_url(url))
    }
}

impl Platform for MacosPlatform {}

impl ViewCapture for MacosPlatform {}

impl ViewFocus for MacosPlatform {
    fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.with_view(id, WebView::focus)
    }

    fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.with_view(id, WebView::focus_parent)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::protocol_handler::ProtocolResponder;
    use crate::protocol_request::ProtocolRequest;

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
        let started = std::thread::spawn(move || MacosPlatform::start(config).map(drop))
            .join()
            .unwrap();
        assert!(matches!(started, Err(Error::NotMainThread)), "refused");
    }
}
