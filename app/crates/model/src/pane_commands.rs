use protocol::{PaneId, PaneState};

use crate::error::{Error, Result};
use crate::layout::{Layout, MAX_TABS};
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::tab::Tab;
use crate::tab_id::TabId;

pub(crate) fn open(
    layout: &mut Layout,
    tab: TabId,
    instance: PaneInstance,
    placement: Option<Placement>,
) -> Result<()> {
    if layout.pane(&instance.pane_id).is_some() {
        return Err(Error::DuplicatePane(instance.pane_id));
    }
    layout.index_of(tab)?;
    if let Some(open_already) = layout.find_pane(&instance.type_id, &instance.params) {
        let existing = open_already.pane_id.clone();
        return reveal(layout, &existing, tab, placement);
    }
    layout.tab_mut(tab)?.open(instance, placement)?;
    layout.active = Some(tab);
    Ok(())
}

fn reveal(
    layout: &mut Layout,
    existing: &PaneId,
    tab: TabId,
    placement: Option<Placement>,
) -> Result<()> {
    let home = layout.index_of_pane(existing)?;
    let home_id = layout.tabs.get(home).map(Tab::id);
    if home_id != Some(tab) {
        return move_to_tab(layout, existing, tab, placement);
    }
    let target = layout.tab_mut(tab)?;
    if let Some(placement) = placement.filter(|placement| &placement.target != existing) {
        target.relocate(existing, &placement)?;
    }
    target.focus(existing)?;
    layout.active = Some(tab);
    Ok(())
}

pub(crate) fn close(layout: &mut Layout, pane: &PaneId) -> Result<Vec<PaneInstance>> {
    let index = layout.index_of_pane(pane)?;
    let tab = layout
        .tabs
        .get_mut(index)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    let instance = tab.remove(pane)?;
    if tab.is_empty() {
        layout.remove_tab_at(index);
    }
    Ok(vec![instance])
}

pub(crate) fn focus(layout: &mut Layout, pane: &PaneId) -> Result<()> {
    let tab = layout.tab_of_mut(pane)?;
    tab.focus(pane)?;
    layout.active = Some(tab.id());
    Ok(())
}

pub(crate) fn relocate(layout: &mut Layout, pane: &PaneId, placement: &Placement) -> Result<()> {
    layout.tab_of_mut(pane)?.relocate(pane, placement)
}

pub(crate) fn swap(layout: &mut Layout, a: &PaneId, b: &PaneId) -> Result<()> {
    layout.tab_of_mut(a)?.swap(a, b)
}

pub(crate) fn set_state(layout: &mut Layout, pane: &PaneId, state: PaneState) -> Result<()> {
    layout.tab_of_mut(pane)?.set_state(pane, state)
}

pub(crate) fn move_to_tab(
    layout: &mut Layout,
    pane: &PaneId,
    to: TabId,
    placement: Option<Placement>,
) -> Result<()> {
    let from = layout.index_of_pane(pane)?;
    let destination = layout.index_of(to)?;
    if from == destination {
        return Err(Error::SameTab(to));
    }
    layout.tab_mut(to)?.check_open(pane, placement.as_ref())?;
    let source = layout
        .tabs
        .get_mut(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    let instance = source.remove(pane)?;
    let emptied = source.is_empty();
    layout.tab_mut(to)?.open(instance, placement)?;
    if emptied {
        layout.remove_tab_at(from);
    }
    layout.active = Some(to);
    Ok(())
}

pub(crate) fn detach(
    layout: &mut Layout,
    pane: &PaneId,
    new_tab: TabId,
    title: String,
) -> Result<()> {
    let from = layout.index_of_pane(pane)?;
    if layout.tab(new_tab).is_some() {
        return Err(Error::DuplicateTab(new_tab));
    }
    if layout.tabs.len() >= MAX_TABS {
        return Err(Error::TooManyTabs { limit: MAX_TABS });
    }
    let source = layout
        .tabs
        .get_mut(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    if source.len() < 2 {
        return Err(Error::OnlyPane(pane.clone()));
    }
    let instance = source.remove(pane)?;
    let mut tab = Tab::new(new_tab, title);
    tab.open(instance, None)?;
    layout.tabs.insert(from + 1, tab);
    layout.active = Some(new_tab);
    Ok(())
}

#[cfg(test)]
#[path = "pane_commands_tests.rs"]
mod tests;
