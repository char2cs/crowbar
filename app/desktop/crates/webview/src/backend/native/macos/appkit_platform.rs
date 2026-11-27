use std::cell::{Cell, RefCell};
use std::collections::HashMap;
use std::rc::Rc;
use std::sync::Arc;

use objc2_foundation::MainThreadMarker;
use wry::WebView;

use crate::backend::host_config::HostConfig;
use crate::backend::native::macos::page::{Page, filling, frame_of};
use crate::backend::native::wry_builder::to_rect;
use crate::backend::platform::Platform;
use crate::backend::view::page_control::PageControl;
use crate::backend::view::view_capture::ViewCapture;
use crate::backend::view::view_cutout::ViewCutout;
use crate::backend::view::view_focus::ViewFocus;
use crate::backend::view::view_layout::ViewLayout;
use crate::error::{Error, Result};
use crate::scheme::ProtocolHandler;
use crate::view::Bounds;
use crate::view::Color;
use crate::view::Corners;
use crate::view::Id;
use crate::view::Kind;
use crate::view::Visibility;

pub(crate) struct AppkitPlatform {
    pub(crate) scheme: String,
    pub(crate) protocol: Arc<dyn ProtocolHandler>,
    pub(crate) views: RefCell<HashMap<Id, Rc<Page>>>,
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

    pub(crate) fn page(
        &self,
        id: Id,
    ) -> Result<Rc<Page>> {
        self.views
            .borrow()
            .get(&id)
            .cloned()
            .ok_or(Error::UnknownWebView(id))
    }

    fn with_view<R>(
        &self,
        id: Id,
        run: impl FnOnce(&WebView) -> wry::Result<R>,
    ) -> Result<R> {
        Ok(run(&self.page(id)?.view)?)
    }
}

impl ViewLayout for AppkitPlatform {
    fn set_bounds(
        &self,
        id: Id,
        bounds: Bounds,
    ) -> Result<()> {
        let page = self.page(id)?;
        page.clip.set_frame(frame_of(bounds));
        Ok(page.view.set_bounds(to_rect(filling(bounds)))?)
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

impl ViewCutout for AppkitPlatform {
    fn set_cutout(
        &self,
        id: Id,
        hole: Option<Bounds>,
    ) -> Result<()> {
        self.page(id)?.set_hole(hole);
        Ok(())
    }

    fn set_corners(
        &self,
        id: Id,
        corners: Corners,
    ) -> Result<()> {
        self.page(id)?.set_corners(corners);
        Ok(())
    }
}

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
        // PLATFORM: macos wry would focus the clip, which types nothing; the shell's view holds it.
        self.page(id)?.clip.hand_keys_to_parent();
        Ok(())
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
