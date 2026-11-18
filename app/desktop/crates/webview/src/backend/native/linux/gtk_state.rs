#[cfg(target_os = "linux")]
mod adapters;
#[cfg(target_os = "linux")]
mod live_view;
#[cfg(target_os = "linux")]
mod placed;
#[cfg(target_os = "linux")]
mod remembered_place;
#[cfg(target_os = "linux")]
mod view_ops;

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use platform::XlibParent;
use protocol::ExtensionId;
use wry::{WebContext, WebViewBuilder, WebViewBuilderExtUnix, WebViewExtUnix};

use crate::backend::host_config::HostConfig;
use crate::backend::native::linux::gtk_state::adapters::browser_data_dir;
use crate::backend::native::linux::gtk_state::live_view::LiveView;
use crate::backend::native::linux::gtk_state::remembered_place::RememberedPlace;
use crate::backend::native::wry_builder::{common_attributes, register_scheme, to_rect};
use crate::error::{Error, Result};
use crate::scheme::ProtocolHandler;
use crate::view::Config;
use crate::view::Id;
use crate::view::Kind;
use crate::view::ParentWindow;

const BROWSER_DIR: &str = "browser";

pub(crate) struct GtkState {
    scheme: String,
    protocol: Arc<dyn ProtocolHandler>,
    data_dir: Option<PathBuf>,
    core_context: WebContext,
    browser_context: WebContext,
    extension_contexts: HashMap<ExtensionId, WebContext>,
    pub(crate) views: HashMap<Id, LiveView>,
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
        id: Id,
        parent: ParentWindow,
        config: &Config,
    ) -> Result<()> {
        let ParentWindow::X11(window) = parent else {
            return Err(Error::UnsupportedParent("a Linux host needs an X11 window"));
        };
        let kind = config.kind.clone();
        let builder = self.builder_for(&kind);
        let id_text = id.to_string();
        let view = common_attributes(builder, &id_text, config)
            .build_as_child(&XlibParent::new(window))?;
        let wanted = to_rect(config.bounds);
        self.views.insert(
            id,
            LiveView {
                view,
                kind,
                place: RememberedPlace::new(wanted),
            },
        );
        Ok(())
    }

    pub(crate) fn clear_browsing_data(&mut self) -> Result<()> {
        let open = self.views.values().find(|live| live.kind == Kind::Browser);
        if let Some(live) = open {
            return Ok(live.view.clear_all_browsing_data()?);
        }
        let Some(dir) = self.data_dir.as_ref().map(|dir| dir.join(BROWSER_DIR)) else {
            return Ok(());
        };
        browser_data_dir::remove(&dir)
    }

    pub(crate) fn destroy(
        &mut self,
        id: Id,
    ) -> Result<()> {
        self.views
            .remove(&id)
            .map(drop)
            .ok_or(Error::UnknownWebView(id))
    }

    pub(crate) fn live(
        &self,
        id: Id,
    ) -> Result<&LiveView> {
        self.views.get(&id).ok_or(Error::UnknownWebView(id))
    }

    fn builder_for<'a>(
        &'a mut self,
        kind: &Kind,
    ) -> WebViewBuilder<'a> {
        let related = self
            .views
            .values()
            .find(|live| live.kind == Kind::Core && *kind == Kind::Core)
            .map(|live| live.view.webview());
        if let Some(anchor) = related {
            return WebViewBuilder::new().with_related_view(anchor);
        }
        if *kind == Kind::Browser {
            return WebViewBuilder::new_with_web_context(&mut self.browser_context);
        }
        let scheme = self.scheme.clone();
        let protocol = Arc::clone(&self.protocol);
        let context = match kind {
            Kind::Core | Kind::Browser => &mut self.core_context,
            Kind::Extension(extension) => {
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
    use crate::scheme::ProtocolRequest;
    use crate::scheme::ProtocolResponse;
    use crate::scheme::{ProtocolHandler, ProtocolResponder};
    use crate::view::Bounds;
    use crate::view::Visibility;

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
    ) -> Config {
        Config {
            url: url.into(),
            bounds,
            kind: Kind::Core,
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
        let refused = state.create(Id(1), ParentWindow::AppKit(1), &config);
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
        let _view = common_attributes(state.builder_for(&Kind::Core), "wv-1", &config)
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
