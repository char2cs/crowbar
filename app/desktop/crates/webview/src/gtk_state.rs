use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::mpsc;

use protocol::ExtensionId;
use webkit2gtk::{SnapshotOptions, SnapshotRegion, WebViewExt as _};
use wry::{WebContext, WebViewBuilder, WebViewBuilderExtUnix, WebViewExtUnix};

use crate::bounds::Bounds;
use crate::builder_support::{common_attributes, register_scheme, to_rect};
use crate::error::{Error, Result};
use crate::host_config::HostConfig;
use crate::live_view::LiveView;
use crate::parent_window::ParentWindow;
use crate::protocol_handler::ProtocolHandler;
use crate::snapshot::Snapshot;
use crate::surface_pixels::snapshot_of;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;
use crate::web_view_kind::WebViewKind;
use crate::xlib_parent::XlibParent;

fn answer(
    reply: &mpsc::Sender<Result<Option<Snapshot>>>,
    result: Result<Option<Snapshot>>,
) {
    if reply.send(result).is_err() {
        tracing::debug!("a snapshot arrived after its caller gave up");
    }
}

pub(crate) struct GtkState {
    scheme: String,
    protocol: Arc<dyn ProtocolHandler>,
    data_dir: Option<PathBuf>,
    core_context: WebContext,
    browser_context: WebContext,
    extension_contexts: HashMap<ExtensionId, WebContext>,
    views: HashMap<WebViewId, LiveView>,
}

impl GtkState {
    pub(crate) fn new(config: HostConfig) -> Self {
        let core_dir = config.data_dir.as_ref().map(|dir| dir.join("core"));
        let browser_dir = config.data_dir.as_ref().map(|dir| dir.join("browser"));
        Self {
            scheme: config.scheme,
            protocol: config.protocol,
            core_context: WebContext::new(core_dir),
            browser_context: WebContext::new(browser_dir),
            data_dir: config.data_dir,
            extension_contexts: HashMap::new(),
            views: HashMap::new(),
        }
    }

    pub(crate) fn create(
        &mut self,
        id: WebViewId,
        parent: ParentWindow,
        config: &WebViewConfig,
    ) -> Result<()> {
        let ParentWindow::X11(window) = parent else {
            return Err(Error::UnsupportedParent("a Linux host needs an X11 window"));
        };
        let kind = config.kind.clone();
        let builder = self.builder_for(&kind);
        let id_text = id.to_string();
        let view =
            common_attributes(builder, &id_text, config).build_as_child(&XlibParent(window))?;
        self.views.insert(id, LiveView { view, kind });
        Ok(())
    }

    pub(crate) fn set_bounds(
        &self,
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()> {
        Ok(self.live(id)?.view.set_bounds(to_rect(bounds))?)
    }

    pub(crate) fn set_visibility(
        &self,
        id: WebViewId,
        visibility: Visibility,
    ) -> Result<()> {
        Ok(self
            .live(id)?
            .view
            .set_visible(visibility == Visibility::Shown)?)
    }

    pub(crate) fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()> {
        Ok(self.live(id)?.view.evaluate_script(script)?)
    }

    pub(crate) fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()> {
        Ok(self.live(id)?.view.load_url(url)?)
    }

    pub(crate) fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        Ok(self.live(id)?.view.focus()?)
    }

    pub(crate) fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        Ok(self.live(id)?.view.focus_parent()?)
    }

    pub(crate) fn snapshot(
        &self,
        id: WebViewId,
        reply: mpsc::Sender<Result<Option<Snapshot>>>,
    ) {
        let page = match self.live(id) {
            Ok(live) => live.view.webview(),
            Err(error) => return answer(&reply, Err(error)),
        };
        page.snapshot(
            SnapshotRegion::Visible,
            SnapshotOptions::NONE,
            None::<&gtk::gio::Cancellable>,
            move |surface| answer(&reply, Ok(surface.ok().and_then(snapshot_of))),
        );
    }

    pub(crate) fn destroy(
        &mut self,
        id: WebViewId,
    ) -> Result<()> {
        self.views
            .remove(&id)
            .map(drop)
            .ok_or(Error::UnknownWebView(id))
    }

    fn live(
        &self,
        id: WebViewId,
    ) -> Result<&LiveView> {
        self.views.get(&id).ok_or(Error::UnknownWebView(id))
    }

    fn builder_for<'a>(
        &'a mut self,
        kind: &WebViewKind,
    ) -> WebViewBuilder<'a> {
        let related = self
            .views
            .values()
            .find(|live| live.kind == WebViewKind::Core && *kind == WebViewKind::Core)
            .map(|live| live.view.webview());
        if let Some(anchor) = related {
            return WebViewBuilder::new().with_related_view(anchor);
        }
        if *kind == WebViewKind::Browser {
            return WebViewBuilder::new_with_web_context(&mut self.browser_context);
        }
        let scheme = self.scheme.clone();
        let protocol = Arc::clone(&self.protocol);
        let context = match kind {
            WebViewKind::Core | WebViewKind::Browser => &mut self.core_context,
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

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::protocol_handler::{ProtocolHandler, ProtocolResponder};
    use crate::protocol_request::ProtocolRequest;

    fn rect(
        x: f64,
        y: f64,
        width: f64,
        height: f64,
    ) -> Bounds {
        Bounds {
            x,
            y,
            width,
            height,
        }
    }

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
    #[ignore = "needs display"]
    fn a_parent_that_is_not_an_x11_window_is_refused() {
        gtk::init().unwrap();
        let mut state = GtkState::new(HostConfig {
            scheme: "crowbar".into(),
            protocol: Arc::new(Silent),
            data_dir: None,
        });
        let config = WebViewConfig {
            url: "about:blank".into(),
            bounds: rect(0.0, 0.0, 1.0, 1.0),
            kind: WebViewKind::Core,
            visibility: Visibility::Hidden,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        };
        let id = WebViewId(1);
        let refused = state.create(id, ParentWindow::AppKit(1), &config);
        assert!(
            matches!(refused, Err(Error::UnsupportedParent(_))),
            "refused"
        );
    }
}
