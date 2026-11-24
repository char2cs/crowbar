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
    use webview::{Bounds, Call, MockPlatform, ParentWindow};

    use super::*;
    use crate::slot::inbox::IpcMessage;

    fn factory(mock: &Rc<MockPlatform>) -> (Factory, async_channel::Receiver<IpcMessage>) {
        let (sink, inbox) = async_channel::unbounded();
        let config = Config {
            scheme: "crowbar".into(),
            host: HostId::new("local").unwrap(),
            max_idle_slots: 1,
            page_handler: None,
        };
        let webviews = webview::Host::with_platform(Rc::<MockPlatform>::clone(mock));
        (
            Factory::new(webviews, config, Arc::new(Gate::new(sink))),
            inbox,
        )
    }

    fn square() -> Bounds {
        Bounds {
            x: 0.0,
            y: 0.0,
            width: 10.0,
            height: 10.0,
        }
    }

    fn request() -> Request {
        Request {
            parent: ParentWindow::X11(3),
            kind: webview::Kind::Core,
            bounds: square(),
        }
    }

    #[test]
    fn slots_are_born_hidden_and_blank_so_nothing_loads_before_the_origin_is_known() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, _inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        assert!(
            matches!(
                mock.calls().first(),
                Some(Call::Create { url, visibility: Visibility::Hidden, parent: ParentWindow::X11(3), .. })
                    if url == BLANK_URL
            ),
            "{:?}",
            mock.calls()
        );
        assert_eq!(slot.id, Id(0), "first slot id");
        assert_eq!(factory.create(request()).unwrap().id, Id(1), "ids advance");
    }

    const SURFACE: Color = Color {
        red: 10,
        green: 20,
        blue: 30,
        alpha: 255,
    };

    fn created_backgrounds(mock: &MockPlatform) -> Vec<Option<Color>> {
        mock.calls()
            .iter()
            .filter_map(|call| match call {
                Call::Create { background, .. } => Some(*background),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn a_pane_view_is_born_in_the_surface_colour_and_a_browser_view_keeps_the_engine_default() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, _inbox) = factory(&mock);
        factory.set_background(SURFACE);
        factory.create(request()).unwrap();
        let mut browser = request();
        browser.kind = webview::Kind::Browser;
        factory.create(browser).unwrap();
        assert_eq!(
            created_backgrounds(&mock),
            vec![Some(SURFACE), None],
            "pane, browser"
        );
    }

    enum Page {
        Unowned,
        Current,
        Stale,
    }

    const CASES: &[(&str, Page, bool)] = &[
        ("nobody owns the slot", Page::Unowned, false),
        ("the owner's current page", Page::Current, true),
        (
            "a page left over from the previous pane",
            Page::Stale,
            false,
        ),
    ];

    fn generation_for(
        page: &Page,
        stale: u64,
        current: u64,
    ) -> u64 {
        if matches!(page, Page::Stale) {
            return stale;
        }
        current
    }

    fn delivered(page: &Page) -> bool {
        let mock = Rc::new(MockPlatform::default());
        let (factory, inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        let stale = slot.owner.assign(PaneId::new("old").unwrap());
        let current = slot.owner.assign(PaneId::new("new").unwrap());
        if matches!(page, Page::Unowned) {
            slot.owner.clear();
        }
        let generation = generation_for(page, stale, current);
        mock.simulate_ipc(slot.handle.id(), &format!("{generation}:hello"))
            .unwrap();
        inbox.try_recv().is_ok()
    }

    #[test]
    fn only_the_current_page_of_the_owner_is_delivered() {
        for (name, page, expected) in CASES {
            assert_eq!(delivered(page), *expected, "{name}");
        }
    }

    #[test]
    fn ipc_is_attributed_to_the_page_generation_of_the_owner() {
        let mock = Rc::new(MockPlatform::default());
        let (factory, inbox) = factory(&mock);
        let slot = factory.create(request()).unwrap();
        let first = slot.owner.assign(PaneId::new("p1").unwrap());
        mock.simulate_ipc(slot.handle.id(), &format!("{first}:hello"))
            .unwrap();
        let message = inbox.try_recv().unwrap();
        assert_eq!(
            (message.pane.as_str(), message.text.as_str()),
            ("p1", "hello"),
            "owner"
        );
        let second = slot.owner.assign(PaneId::new("p2").unwrap());
        mock.simulate_ipc(slot.handle.id(), &format!("{first}:from the old page"))
            .unwrap();
        assert!(
            inbox.try_recv().is_err(),
            "the previous pane's page is stale"
        );
        mock.simulate_ipc(slot.handle.id(), &format!("{second}:again"))
            .unwrap();
        assert_eq!(
            inbox.try_recv().unwrap().pane.as_str(),
            "p2",
            "the new page"
        );
    }
}
