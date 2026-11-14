use crate::branch::Branch;
use crate::error::{Error, Result};
use crate::layout::{Layout, MAX_TABS};
use crate::pane_instance::PaneInstance;
use crate::tab::Tab;
use crate::tab_id::TabId;

pub(crate) fn add(layout: &mut Layout, id: TabId, title: String) -> Result<()> {
    if layout.tab(id).is_some() {
        return Err(Error::DuplicateTab(id));
    }
    if layout.tabs.len() >= MAX_TABS {
        return Err(Error::TooManyTabs { limit: MAX_TABS });
    }
    layout.tabs.push(Tab::new(id, title));
    layout.active = Some(id);
    Ok(())
}

pub(crate) fn close(layout: &mut Layout, id: TabId) -> Result<Vec<PaneInstance>> {
    let index = layout.index_of(id)?;
    let mut removed = layout.remove_tab_at(index).ok_or(Error::TabNotFound(id))?;
    Ok(removed.take_instances().into_values().collect())
}

pub(crate) fn activate(layout: &mut Layout, id: TabId) -> Result<()> {
    layout.index_of(id)?;
    layout.active = Some(id);
    Ok(())
}

pub(crate) fn move_to(layout: &mut Layout, id: TabId, to_index: usize) -> Result<()> {
    let index = layout.index_of(id)?;
    if to_index >= layout.tabs.len() {
        return Err(Error::IndexOutOfRange {
            index: to_index,
            len: layout.tabs.len(),
        });
    }
    let tab = layout.tabs.remove(index);
    layout.tabs.insert(to_index, tab);
    Ok(())
}

pub(crate) fn rename(layout: &mut Layout, id: TabId, title: String) -> Result<()> {
    layout.tab_mut(id)?.set_title(title);
    Ok(())
}

pub(crate) fn equalize(layout: &mut Layout, id: TabId, path: &[Branch]) -> Result<()> {
    layout.tab_mut(id)?.equalize(path)
}

#[cfg(test)]
#[path = "tab_commands_tests.rs"]
mod tests;
