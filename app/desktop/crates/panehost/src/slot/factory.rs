use std::cell::Cell;
use std::sync::Arc;

use webview::{Bounds, Color, IpcHandler, Visibility};

use crate::error::Result;
use crate::slot::Slot;
use crate::slot::bootstrap::bootstrap_script;
use crate::slot::config::Config;
use crate::slot::id::Id;
use crate::slot::inbox::Gate;
use crate::slot::inbox::drop_handler;
use crate::slot::inbox::open_envelope;
use crate::slot::inbox::press_handler;
use crate::slot::inbox::{page_handler, pick_handler};
use crate::slot::owner::Owner;
use crate::slot::request::{BLANK_URL, Request};

pub(crate) struct Factory {
    webviews: webview::Host,
    config: Config,
    gate: Arc<Gate>,
    next: Cell<u64>,
    background: Cell<Option<Color>>,
}

impl Factory {
    pub(crate) fn new(
        webviews: webview::Host,
        config: Config,
        gate: Arc<Gate>,
    ) -> Self {
        Self {
            webviews,
            config,
            gate,
            next: Cell::new(0),
            background: Cell::new(None),
        }
    }

    pub(crate) fn set_background(
        &self,
        color: Color,
    ) {
        self.background.set(Some(color));
    }

    pub(crate) fn clear_browsing_data(&self) -> webview::Result<()> {
        self.webviews.clear_browsing_data()
    }

    pub(crate) fn desktop(&self) -> webview::Desktop {
        self.webviews.desktop()
    }

    pub(crate) fn settle(&self) -> webview::Result<()> {
        self.webviews.settle()
    }

    pub(crate) fn create(
        &self,
        request: Request,
    ) -> Result<Slot> {
        let Request {
            parent,
            kind,
            bounds,
        } = request;
        let id = Id(self.next.get());
        self.next.set(id.0 + 1);
        let owner = Owner::default();
        let config = self.config_for(&kind, bounds, &owner);
        let handle = self.webviews.create_in(parent, config)?;
        Ok(Slot {
            id,
            kind,
            handle,
            owner,
        })
    }

    fn config_for(
        &self,
        kind: &webview::Kind,
        bounds: Bounds,
        owner: &Owner,
    ) -> webview::Config {
        if *kind == webview::Kind::Browser {
            return self.browser_config(bounds, owner);
        }
        self.pane_config((kind.clone(), bounds), owner)
    }

    fn browser_config(
        &self,
        bounds: Bounds,
        owner: &Owner,
    ) -> webview::Config {
        webview::Config {
            url: BLANK_URL.into(),
            bounds,
            kind: webview::Kind::Browser,
            visibility: Visibility::Hidden,
            background: None,
            initialization_script: None,
            on_ipc: pick_handler(owner.clone(), self.gate.sink()),
            on_page: Some(press_handler(
                Some(page_handler(owner.clone(), self.gate.sink())),
                owner.clone(),
                self.gate.sink(),
            )),
        }
    }

    fn pane_config(
        &self,
        (kind, bounds): (webview::Kind, Bounds),
        owner: &Owner,
    ) -> webview::Config {
        webview::Config {
            url: BLANK_URL.into(),
            bounds,
            kind,
            visibility: Visibility::Hidden,
            background: self.background.get(),
            initialization_script: Some(bootstrap_script(&self.config.scheme, &self.config.host)),
            on_ipc: self.ipc_for(owner),
            on_page: Some(press_handler(
                Some(drop_handler(
                    self.config.page_handler.clone(),
                    owner.clone(),
                    self.gate.sink(),
                )),
                owner.clone(),
                self.gate.sink(),
            )),
        }
    }

    fn ipc_for(
        &self,
        owner: &Owner,
    ) -> IpcHandler {
        let owner = owner.clone();
        let gate = Arc::clone(&self.gate);
        Arc::new(move |text| deliver(&owner, &gate, &text))
    }
}

fn deliver(
    owner: &Owner,
    gate: &Gate,
    text: &str,
) {
    let Some((generation, frame)) = open_envelope(text) else {
        return;
    };
    let Some(pane) = owner.resolve(generation) else {
        return;
    };
    gate.admit(pane, frame);
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use protocol::{HostId, PaneId};
    use webview::{Call, MockPlatform, ParentWindow};

    use super::*;
    use crate::slot::IpcMessage;
    use crate::test_support::bounds;

    fn factory(mock: &Rc<MockPlatform>) -> (Factory, async_channel::Receiver<IpcMessage>) {
        let (sink, inbox) = async_channel::unbounded();
        let config = Config {
            scheme: "crowbar".into(),
            host: HostId::new("local").unwrap(),
            max_idle_slots: 1,
            page_handler: None,
        };
        let webviews = webview::Host::with_platform(Rc::<MockPlatform>::clone(mock));
        let gate = Arc::new(Gate::new(sink));
        (Factory::new(webviews, config, gate), inbox)
    }

    fn request(kind: webview::Kind) -> Request {
        let parent = ParentWindow::X11(3);
        let bounds = bounds();
        Request {
            parent,
            kind,
            bounds,
        }
    }

    #[test]
    fn a_pane_view_is_born_in_the_surface_colour_and_a_browser_view_keeps_the_engine_default() {
        const SURFACE: Color = Color {
            red: 10,
            green: 20,
            blue: 30,
            alpha: 255,
        };
        let mock = Rc::new(MockPlatform::default());
        let (factory, _inbox) = factory(&mock);
        factory.set_background(SURFACE);
        factory.create(request(webview::Kind::Core)).unwrap();
        factory.create(request(webview::Kind::Browser)).unwrap();
        let born: Vec<Option<Color>> = mock
            .calls()
            .iter()
            .filter_map(|call| match call {
                Call::Create { background, .. } => Some(*background),
                _ => None,
            })
            .collect();
        assert_eq!(born, vec![Some(SURFACE), None], "pane, browser");
    }

    #[test]
    fn ipc_is_attributed_to_the_current_page_of_the_owner_only() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, inbox) = factory(&mock);
        let slot = factory.create(request(webview::Kind::Core)).unwrap();
        let post = |generation: u64, text: &str| {
            let enveloped = format!("{generation}:{text}");
            mock.simulate_ipc(slot.handle.id(), &enveloped).unwrap();
            inbox
                .try_recv()
                .ok()
                .map(|message| (message.pane.to_string(), message.text))
        };
        let first = slot.owner.assign(PaneId::new("p1").unwrap());
        assert_eq!(
            post(first, "hello"),
            Some(("p1".into(), "hello".into())),
            "owner"
        );
        let second = slot.owner.assign(PaneId::new("p2").unwrap());
        assert_eq!(
            post(first, "old"),
            None,
            "the previous pane's page is stale"
        );
        assert_eq!(
            post(second, "again"),
            Some(("p2".into(), "again".into())),
            "new page"
        );
        slot.owner.clear();
        assert_eq!(post(second, "late"), None, "nobody owns the slot");
    }
}
