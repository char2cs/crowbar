use std::cell::RefCell;
use std::collections::HashMap;
use std::ptr::NonNull;
use std::rc::Rc;
use std::sync::Arc;

use objc2_foundation::MainThreadMarker;
use protocol::ExtensionId;
use wry::{WebView, WebViewBuilder, WebViewBuilderExtDarwin};

use crate::appkit_parent::AppkitParent;
use crate::bounds::Bounds;
use crate::builder_support::{common_attributes, register_scheme, to_rect};
use crate::error::{Error, Result};
use crate::host_config::HostConfig;
use crate::parent_window::ParentWindow;
use crate::platform::Platform;
use crate::protocol_handler::ProtocolHandler;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;
use crate::web_view_kind::WebViewKind;

pub(crate) struct MacosPlatform {
    scheme: String,
    protocol: Arc<dyn ProtocolHandler>,
    views: RefCell<HashMap<WebViewId, Rc<WebView>>>,
}

impl MacosPlatform {
    pub(crate) fn start(config: HostConfig) -> Result<Self> {
        MainThreadMarker::new().ok_or(Error::NotMainThread)?;
        Ok(Self {
            scheme: config.scheme,
            protocol: config.protocol,
            views: RefCell::new(HashMap::new()),
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
            // PLATFORM: no custom scheme here, so a browser page cannot reach the pane protocol; the fixed identifier gives it a data store apart from core and every extension.
            WebViewKind::Browser => WebViewBuilder::new().with_data_store_identifier(BROWSER_STORE),
        }
    }
}

impl Platform for MacosPlatform {
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
        let view = NonNull::new(std::ptr::with_exposed_provenance_mut(address))
            .ok_or(Error::UnsupportedParent("null NSView"))?;
        let builder = self.builder_for(&config.kind);
        let id_text = id.to_string();
        let web_view =
            common_attributes(builder, &id_text, &config).build_as_child(&AppkitParent(view))?;
        self.views.borrow_mut().insert(id, Rc::new(web_view));
        Ok(())
    }

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

    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.views
            .borrow_mut()
            .remove(&id)
            .map(drop)
            .ok_or(Error::UnknownWebView(id))
    }
}

const STORE_ID_BYTES: usize = 16;
const BROWSER_STORE: [u8; STORE_ID_BYTES] = *b"crowbar.browser!";

fn store_identifier(extension: &ExtensionId) -> [u8; STORE_ID_BYTES] {
    const OFFSET_BASIS: u128 = 0x6c62_2727_e07b_b014_62b8_2175_6295_c58d;
    const PRIME: u128 = 0x0000_0000_0100_0000_0000_0000_0000_013b;
    let hash = extension.as_str().bytes().fold(OFFSET_BASIS, |hash, byte| {
        (hash ^ u128::from(byte)).wrapping_mul(PRIME)
    });
    hash.to_be_bytes()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extensions_get_distinct_stable_store_identifiers() {
        let git = ExtensionId::new("git").unwrap();
        let draw = ExtensionId::new("draw").unwrap();
        assert_eq!(store_identifier(&git), store_identifier(&git), "stable");
        assert_ne!(store_identifier(&git), store_identifier(&draw), "distinct");
    }
}
