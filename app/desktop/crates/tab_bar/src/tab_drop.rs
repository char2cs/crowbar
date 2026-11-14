use model::{Command, DragSubject, TabId};

use crate::new_tab::NewTab;
use crate::span::Span;
use crate::tab_view::TabView;

const EDGE: f32 = 0.25;

/// Where a chat dragged from the sidebar or a pane handle would land on the strip.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TabDrop {
    /// Between tabs: a tab of its own at this index.
    Reorder(usize),
    /// On the middle of a tab: merged into that tab as a split.
    Merge(TabId),
}

impl TabDrop {
    /// The drop under pointer `x` among tab `spans` and their `tabs`: the outer quarter of a slot
    /// is the gap before or after it, the middle half is the tab itself.
    #[must_use]
    pub fn at(x: f32, spans: &[Span], tabs: &[TabId]) -> Option<Self> {
        let last = spans.len().checked_sub(1)?;
        let index = spans.iter().position(|span| x < span.end()).unwrap_or(last);
        let span = spans.get(index)?;
        let across = (x - span.x) / span.width;
        if across < EDGE {
            return Some(Self::Reorder(index));
        }
        if across > 1.0 - EDGE {
            return Some(Self::Reorder(index + 1));
        }
        tabs.get(index).copied().map(Self::Merge)
    }

    /// The commands that carry out the drop of `subject`; `new_tab` names the tab a pane or chat
    /// gets when it does not merge. A tab drag is the strip's own reorder and yields nothing.
    #[must_use]
    pub fn commands(
        &self,
        subject: &DragSubject,
        new_tab: NewTab,
        tabs: &[TabView],
    ) -> Vec<Command> {
        match (subject, self) {
            (DragSubject::Pane(pane), Self::Merge(to)) => {
                vec![Command::MovePaneToTab {
                    pane: pane.clone(),
                    to: *to,
                    placement: None,
                }]
            },
            (DragSubject::Open(instance), Self::Merge(tab)) => {
                vec![Command::OpenPane {
                    tab: *tab,
                    instance: instance.clone(),
                    placement: None,
                }]
            },
            (DragSubject::Pane(pane), Self::Reorder(index)) => detach(pane, *index, new_tab, tabs),
            (DragSubject::Open(instance), Self::Reorder(index)) => vec![
                Command::AddTab {
                    id: new_tab.id,
                    title: new_tab.title,
                },
                Command::OpenPane {
                    tab: new_tab.id,
                    instance: instance.clone(),
                    placement: None,
                },
                Command::MoveTab {
                    tab: new_tab.id,
                    to_index: (*index).min(tabs.len()),
                },
            ],
            (DragSubject::Tab(_), _) => Vec::new(),
        }
    }
}

fn detach(pane: &protocol::PaneId, gap: usize, new_tab: NewTab, tabs: &[TabView]) -> Vec<Command> {
    let Some(position) = tabs
        .iter()
        .position(|tab| tab.members.iter().any(|member| &member.pane == pane))
    else {
        return Vec::new();
    };
    let Some(owner) = tabs.get(position) else {
        return Vec::new();
    };
    if owner.members.len() < 2 {
        let to_index = if gap > position { gap - 1 } else { gap };
        return vec![Command::MoveTab {
            tab: owner.id,
            to_index,
        }];
    }
    vec![
        Command::DetachPane {
            pane: pane.clone(),
            new_tab: new_tab.id,
            title: new_tab.title,
        },
        Command::MoveTab {
            tab: new_tab.id,
            to_index: gap.min(tabs.len()),
        },
    ]
}

#[cfg(test)]
#[path = "tab_drop_tests.rs"]
mod tests;
