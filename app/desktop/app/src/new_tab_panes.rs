use std::cell::RefCell;
use std::collections::{BTreeMap, BTreeSet};

use gpui::{AnyView, App, AppContext as _, Entity};
use layout::{Layout, Tab};
use protocol::{PaneId, PaneTitle};

use crate::core_panes::NEW_TAB;
use crate::new_tab_action::NewTabAction;
use crate::new_tab_view::NewTabView;
use crate::shell_request::ShellRequest;
use crate::titles::Titles;

const LABEL: &str = "New Tab";

pub(crate) struct NewTabPanes {
    ids: RefCell<BTreeSet<PaneId>>,
    views: RefCell<BTreeMap<PaneId, Entity<NewTabView>>>,
    requests: async_channel::Sender<ShellRequest>,
}

impl NewTabPanes {
    pub(crate) fn new(requests: async_channel::Sender<ShellRequest>) -> Self {
        Self {
            ids: RefCell::default(),
            views: RefCell::default(),
            requests,
        }
    }

    pub(crate) fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.ids.borrow().contains(pane)
    }

    pub(crate) fn follow(
        &self,
        layout: &Layout,
        titles: &Titles,
    ) {
        let open: BTreeSet<PaneId> = layout
            .tabs()
            .iter()
            .flat_map(Tab::pane_ids)
            .filter(|pane| is_new_tab(layout, pane))
            .collect();
        self.views
            .borrow_mut()
            .retain(|pane, _| open.contains(pane));
        for pane in open.iter().filter(|pane| titles.get(pane).is_none()) {
            name(titles, pane);
        }
        *self.ids.borrow_mut() = open;
    }

    pub(crate) fn view(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) -> Option<AnyView> {
        if !self.contains(pane) {
            return None;
        }
        let existing = self.views.borrow().get(pane).cloned();
        let view = existing.unwrap_or_else(|| self.create(pane, cx));
        Some(view.into())
    }

    fn create(
        &self,
        pane: &PaneId,
        cx: &mut App,
    ) -> Entity<NewTabView> {
        let view = cx.new(|_| NewTabView {
            actions: NewTabAction::ALL,
        });
        let (requests, owner) = (self.requests.clone(), pane.clone());
        cx.subscribe(&view, move |_, action: &NewTabAction, _| {
            forward(&requests, &owner, *action);
        })
        .detach();
        self.views.borrow_mut().insert(pane.clone(), view.clone());
        view
    }
}

fn name(
    titles: &Titles,
    pane: &PaneId,
) {
    if let Ok(title) = PaneTitle::new(LABEL) {
        titles.set(pane.clone(), title);
    }
}

fn forward(
    requests: &async_channel::Sender<ShellRequest>,
    pane: &PaneId,
    action: NewTabAction,
) {
    let request = ShellRequest::NewTabChoice {
        pane: pane.clone(),
        action,
    };
    if requests.try_send(request).is_err() {
        tracing::debug!("the shell is closing; a New Tab choice goes unseen");
    }
}

fn is_new_tab(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    let type_id = layout.pane(pane).map(|instance| instance.type_id.as_str());
    type_id == Some(NEW_TAB)
}

#[cfg(test)]
mod tests {
    use layout::{Command, PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;

    fn instance(
        index: usize,
        type_id: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(format!("p{index}")).unwrap(),
            type_id: PaneTypeId::new(type_id).unwrap(),
            params: Params::default().with("n", index.to_string()).unwrap(),
            state: PaneState::default(),
        }
    }

    fn open(
        layout: &mut Layout,
        instance: PaneInstance,
    ) {
        let command = Command::OpenPane {
            tab: TabId(1),
            instance,
            placement: None,
        };
        layout.apply(command).unwrap();
    }

    fn layout_with(types: &[&str]) -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: String::new(),
            })
            .unwrap();
        for (index, type_id) in types.iter().enumerate() {
            open(&mut layout, instance(index, type_id));
        }
        layout
    }

    #[test]
    fn only_new_tab_panes_are_followed_and_they_are_titled_new_tab() {
        let panes = NewTabPanes::new(async_channel::unbounded().0);
        let titles = Titles::default();
        panes.follow(&layout_with(&["core.chat", NEW_TAB]), &titles);
        let (chat, page) = (PaneId::new("p0").unwrap(), PaneId::new("p1").unwrap());
        assert!(!panes.contains(&chat), "a chat is not a page");
        assert!(panes.contains(&page), "the page is followed");
        assert_eq!(titles.get(&page).map(|t| t.to_string()), Some(LABEL.into()));
        panes.follow(&layout_with(&["core.chat"]), &titles);
        assert!(!panes.contains(&page), "a closed page is forgotten");
    }
}
