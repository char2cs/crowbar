use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::mpsc;

use protocol::ExtensionId;
use webkit2gtk::{SnapshotOptions, SnapshotRegion, WebViewExt as _};
use wry::{WebContext, WebViewBuilder, WebViewBuilderExtUnix, WebViewExtUnix};

use crate::bounds::Bounds;
use crate::builder_support::{common_attributes, register_scheme, to_rect};
use crate::color::Color;
use crate::error::{Error, Result};
use crate::host_config::HostConfig;
use crate::live_view::LiveView;
use crate::parent_window::ParentWindow;
use crate::placed::show_where_it_was;
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

const BROWSER_DIR: &str = "browser";

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
        let browser_dir = config.data_dir.as_ref().map(|dir| dir.join(BROWSER_DIR));
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
        let view = &self.live(id)?.view;
        match visibility {
            Visibility::Hidden => Ok(view.set_visible(false)?),
            Visibility::Shown => show_where_it_was(view),
        }
    }

    pub(crate) fn set_background(
        &self,
        id: WebViewId,
        color: Color,
    ) -> Result<()> {
        Ok(self.live(id)?.view.set_background_color(color.rgba())?)
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

    pub(crate) fn clear_browsing_data(&mut self) -> Result<()> {
        let open = self
            .views
            .values()
            .find(|live| live.kind == WebViewKind::Browser);
        if let Some(live) = open {
            return Ok(live.view.clear_all_browsing_data()?);
        }
        let Some(dir) = self.data_dir.as_ref().map(|dir| dir.join(BROWSER_DIR)) else {
            return Ok(());
        };
        match std::fs::remove_dir_all(&dir) {
            Err(error) if error.kind() != std::io::ErrorKind::NotFound => {
                Err(Error::PlatformThread(error.to_string()))
            },
            _ => Ok(()),
        }
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
    use std::sync::mpsc;
    use std::time::{Duration, Instant};

    use gtk::prelude::{ContainerExt as _, WidgetExt as _};

    use super::*;
    use crate::protocol_handler::{ProtocolHandler, ProtocolResponder};
    use crate::protocol_request::ProtocolRequest;
    use crate::protocol_response::ProtocolResponse;

    const SAVE_PAGE: &str =
        "<script>fetch('crowbar://app/save', { method: 'PUT', body: 'payload' })</script>";
    const PAGE_WAIT: Duration = Duration::from_secs(30);

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

    struct Recording(mpsc::Sender<ProtocolRequest>);

    impl ProtocolHandler for Recording {
        fn handle(
            &self,
            request: ProtocolRequest,
            respond: ProtocolResponder,
        ) {
            let body = [SAVE_PAGE, "{}"][usize::from(!request.uri.ends_with("/page"))];
            self.0.send(request).unwrap();
            respond(ProtocolResponse {
                status: 200,
                headers: vec![("content-type".into(), "text/html".into())],
                body: body.as_bytes().to_vec(),
            });
        }
    }

    fn state_serving(protocol: Arc<dyn ProtocolHandler>) -> GtkState {
        GtkState::new(HostConfig {
            scheme: "crowbar".into(),
            protocol,
            data_dir: None,
        })
    }

    fn config_at(
        url: &str,
        bounds: Bounds,
    ) -> WebViewConfig {
        WebViewConfig {
            url: url.into(),
            bounds,
            kind: WebViewKind::Core,
            visibility: Visibility::Shown,
            background: None,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        }
    }

    fn pump_once(
        requests: &mpsc::Receiver<ProtocolRequest>,
        deadline: Instant,
    ) -> Option<ProtocolRequest> {
        gtk::main_iteration_do(true);
        let saved = requests.try_iter().find(|request| request.method == "PUT");
        assert!(
            saved.is_some() || Instant::now() < deadline,
            "the page's PUT never reached the protocol handler"
        );
        saved
    }

    fn pump_until_save(requests: &mpsc::Receiver<ProtocolRequest>) -> ProtocolRequest {
        gtk::glib::timeout_add_local_once(PAGE_WAIT, || {});
        let deadline = Instant::now() + PAGE_WAIT;
        std::iter::repeat_with(|| pump_once(requests, deadline))
            .flatten()
            .next()
            .unwrap()
    }

    fn a_parent_that_is_not_an_x11_window_is_refused() {
        let mut state = state_serving(Arc::new(Silent));
        let config = config_at("about:blank", rect(0.0, 0.0, 1.0, 1.0));
        let refused = state.create(WebViewId(1), ParentWindow::AppKit(1), &config);
        assert!(
            matches!(refused, Err(Error::UnsupportedParent(_))),
            "refused"
        );
    }

    fn a_page_request_body_reaches_the_protocol_handler() {
        let (sender, requests) = mpsc::channel();
        let mut state = state_serving(Arc::new(Recording(sender)));
        let config = config_at("crowbar://app/page", rect(0.0, 0.0, 100.0, 100.0));
        let host = gtk::Fixed::new();
        let _view = common_attributes(state.builder_for(&WebViewKind::Core), "wv-1", &config)
            .build_gtk(&host)
            .unwrap();
        let window = gtk::Window::new(gtk::WindowType::Toplevel);
        window.add(&host);
        window.show_all();
        assert_eq!(pump_until_save(&requests).body, b"payload", "the PUT body");
    }

    #[test]
    #[ignore = "needs display"]
    fn live_gtk_web_views() {
        gtk::init().unwrap();
        a_parent_that_is_not_an_x11_window_is_refused();
        a_page_request_body_reaches_the_protocol_handler();
    }
}
