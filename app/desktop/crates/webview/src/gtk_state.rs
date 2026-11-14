use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use protocol::ExtensionId;
use wry::{WebContext, WebView, WebViewBuilder, WebViewBuilderExtUnix, WebViewExtUnix};

use crate::bounds::Bounds;
use crate::builder_support::{common_attributes, register_scheme, to_rect};
use crate::error::{Error, Result};
use crate::host_config::HostConfig;
use crate::parent_window::ParentWindow;
use crate::protocol_handler::ProtocolHandler;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;
use crate::web_view_kind::WebViewKind;
use crate::xlib_parent::XlibParent;

struct LiveView {
    view: WebView,
    kind: WebViewKind,
}

/// Everything the GTK thread owns: web contexts and live views. Never leaves that thread.
pub(crate) struct GtkState {
    scheme: String,
    protocol: Arc<dyn ProtocolHandler>,
    data_dir: Option<PathBuf>,
    core_context: WebContext,
    extension_contexts: HashMap<ExtensionId, WebContext>,
    views: HashMap<WebViewId, LiveView>,
}

impl GtkState {
    pub(crate) fn new(config: HostConfig) -> Self {
        let core_dir = config.data_dir.as_ref().map(|dir| dir.join("core"));
        Self {
            scheme: config.scheme,
            protocol: config.protocol,
            core_context: WebContext::new(core_dir),
            data_dir: config.data_dir,
            extension_contexts: HashMap::new(),
            views: HashMap::new(),
        }
    }

    pub(crate) fn create(
        &mut self,
        id: WebViewId,
        parent: ParentWindow,
        config: WebViewConfig,
    ) -> Result<()> {
        let ParentWindow::X11(window) = parent else {
            return Err(Error::UnsupportedParent("a Linux host needs an X11 window"));
        };
        let kind = config.kind.clone();
        let builder = self.builder_for(&kind);
        let id_text = id.to_string();
        let view =
            common_attributes(builder, &id_text, &config).build_as_child(&XlibParent(window))?;
        self.views.insert(id, LiveView { view, kind });
        Ok(())
    }

    pub(crate) fn set_bounds(&self, id: WebViewId, bounds: Bounds) -> Result<()> {
        Ok(self.live(id)?.view.set_bounds(to_rect(bounds))?)
    }

    pub(crate) fn set_visibility(&self, id: WebViewId, visibility: Visibility) -> Result<()> {
        Ok(self
            .live(id)?
            .view
            .set_visible(visibility == Visibility::Shown)?)
    }

    pub(crate) fn eval(&self, id: WebViewId, script: &str) -> Result<()> {
        Ok(self.live(id)?.view.evaluate_script(script)?)
    }

    pub(crate) fn navigate(&self, id: WebViewId, url: &str) -> Result<()> {
        Ok(self.live(id)?.view.load_url(url)?)
    }

    pub(crate) fn focus(&self, id: WebViewId) -> Result<()> {
        Ok(self.live(id)?.view.focus()?)
    }

    pub(crate) fn focus_parent(&self, id: WebViewId) -> Result<()> {
        Ok(self.live(id)?.view.focus_parent()?)
    }

    pub(crate) fn destroy(&mut self, id: WebViewId) -> Result<()> {
        self.views
            .remove(&id)
            .map(drop)
            .ok_or(Error::UnknownWebView(id))
    }

    fn live(&self, id: WebViewId) -> Result<&LiveView> {
        self.views.get(&id).ok_or(Error::UnknownWebView(id))
    }

    fn builder_for<'a>(&'a mut self, kind: &WebViewKind) -> WebViewBuilder<'a> {
        let related = self
            .views
            .values()
            .find(|live| live.kind == WebViewKind::Core && *kind == WebViewKind::Core)
            .map(|live| live.view.webview());
        if let Some(anchor) = related {
            return WebViewBuilder::new().with_related_view(anchor);
        }
        let scheme = self.scheme.clone();
        let protocol = Arc::clone(&self.protocol);
        let context = match kind {
            WebViewKind::Core => &mut self.core_context,
            WebViewKind::Extension(extension) => {
                let dir = self
                    .data_dir
                    .as_ref()
                    .map(|dir| dir.join("ext").join(extension.as_str()));
                self.extension_contexts
                    .entry(extension.clone())
                    .or_insert_with(|| WebContext::new(dir))
            },
        };
        let needs_scheme = !context.is_custom_protocol_registered(&scheme);
        let builder = WebViewBuilder::new_with_web_context(context);
        if needs_scheme {
            return register_scheme(builder, &scheme, protocol);
        }
        builder
    }
}
