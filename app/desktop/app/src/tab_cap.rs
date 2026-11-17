use model::{Command, Layout, PaneInstance};
use protocol::PaneId;

use crate::core_panes::{CHAT, TERMINAL};
use crate::dirty_panes::DirtyPanes;

const ENGINE_LIMIT: u8 = 10;

pub(crate) fn limit(setting: Option<f64>) -> usize {
    let wanted = setting.unwrap_or_else(|| f64::from(ENGINE_LIMIT));
    let cap = (1..=ENGINE_LIMIT)
        .rev()
        .find(|cap| f64::from(*cap) <= wanted)
        .unwrap_or(ENGINE_LIMIT);
    usize::from(cap)
}

fn opened(command: &Command) -> Option<&PaneInstance> {
    match command {
        Command::OpenPane { instance, .. } | Command::OpenPaneInGroup { instance, .. } => {
            Some(instance)
        },
        _ => None,
    }
}

fn counts(instance: &PaneInstance) -> bool {
    instance.type_id.as_str() != CHAT
}

fn owner(instance: &PaneInstance) -> Option<&str> {
    instance.params.get("threadId")
}

fn visible(
    layout: &Layout,
    pane: &PaneId,
) -> bool {
    let Some(tab) = layout.tab_of(pane) else {
        return false;
    };
    let showing = tab
        .group_of(pane)
        .is_some_and(|group| group.active() == pane);
    showing || tab.focused() == Some(pane)
}

fn evictable(
    layout: &Layout,
    instance: &PaneInstance,
    flags: &DirtyPanes,
) -> bool {
    instance.type_id.as_str() != TERMINAL
        && !flags.contains(&instance.pane_id)
        && !visible(layout, &instance.pane_id)
}

fn held_with<'a>(
    layout: &'a Layout,
    incoming: &PaneInstance,
    chosen: &[PaneId],
) -> Vec<&'a PaneInstance> {
    layout
        .tabs()
        .iter()
        .flat_map(model::Tab::pane_ids)
        .filter_map(|pane| layout.pane(&pane))
        .filter(|found| counts(found) && owner(found) == owner(incoming))
        .filter(|found| !chosen.contains(&found.pane_id))
        .collect()
}

fn victim_in(
    layout: &Layout,
    held: &[&PaneInstance],
    flags: &DirtyPanes,
) -> Option<PaneId> {
    held.iter()
        .find(|found| evictable(layout, found, flags))
        .map(|found| found.pane_id.clone())
}

pub(crate) fn evictions(
    layout: &Layout,
    commands: &[Command],
    limit: usize,
    flags: &DirtyPanes,
) -> Vec<PaneId> {
    commands
        .iter()
        .filter_map(opened)
        .filter(|incoming| counts(incoming))
        .fold(Vec::new(), |mut chosen, incoming| {
            let held = held_with(layout, incoming, &chosen);
            chosen.extend(victim_when_full(layout, &held, limit, flags));
            chosen
        })
}

fn victim_when_full(
    layout: &Layout,
    held: &[&PaneInstance],
    limit: usize,
    flags: &DirtyPanes,
) -> Option<PaneId> {
    if held.len() < limit {
        return None;
    }
    victim_in(layout, held, flags)
}

#[cfg(test)]
mod tests {
    use model::TabId;
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;
    use crate::core_panes::{EDITOR, chat_params};

    fn instance(
        id: &str,
        type_id: &str,
        thread: &str,
    ) -> PaneInstance {
        let params = match type_id {
            CHAT => chat_params(thread).unwrap(),
            _ => Params::default()
                .with("threadId", thread)
                .unwrap()
                .with("path", id)
                .unwrap(),
        };
        PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: PaneTypeId::new(type_id).unwrap(),
            params,
            state: PaneState::default(),
        }
    }

    fn open(
        id: &str,
        type_id: &str,
        thread: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(1),
            instance: instance(id, type_id, thread),
            placement: None,
        }
    }

    fn layout_of(panes: &[(&str, &str, &str)]) -> Layout {
        let mut layout = Layout::default();
        let add = Command::AddTab {
            id: TabId(1),
            title: String::new(),
        };
        layout.apply(add).unwrap();
        for (id, type_id, thread) in panes {
            let opened = open_in_group(&layout, id, type_id, thread);
            layout.apply(opened).unwrap();
        }
        layout
    }

    fn open_in_group(
        layout: &Layout,
        id: &str,
        type_id: &str,
        thread: &str,
    ) -> Command {
        let tab = layout.active_tab().unwrap();
        let stacked = tab.pane_ids().into_iter().next();
        match stacked {
            Some(onto) if type_id != CHAT => Command::OpenPaneInGroup {
                instance: instance(id, type_id, thread),
                onto,
                index: None,
            },
            _ => open(id, type_id, thread),
        }
    }

    #[test]
    fn the_setting_only_lowers_the_engine_limit() {
        let cases = [
            (None, 10),
            (Some(0.0), 10),
            (Some(-3.0), 10),
            (Some(4.0), 4),
            (Some(100.0), 10),
        ];
        for (setting, expected) in cases {
            assert_eq!(limit(setting), expected, "{setting:?}");
        }
    }

    #[test]
    fn the_first_hidden_clean_tab_of_the_chat_goes_when_the_limit_is_reached() {
        let layout = layout_of(&[
            ("c", CHAT, "t"),
            ("e1", EDITOR, "t"),
            ("e2", EDITOR, "t"),
            ("e3", EDITOR, "t"),
            ("e4", EDITOR, "t"),
        ]);
        let flags = DirtyPanes::default();
        let incoming = [open("e5", EDITOR, "t")];
        let gone = evictions(&layout, &incoming, 4, &flags);
        assert_eq!(gone.len(), 1, "one tab makes room");
        assert!(!visible(&layout, &gone[0]), "never the tab being looked at");
        assert!(
            evictions(&layout, &incoming, 5, &flags).is_empty(),
            "room left"
        );
    }

    #[test]
    fn dirty_terminal_and_other_chats_tabs_are_never_evicted() {
        let layout = layout_of(&[
            ("c", CHAT, "t"),
            ("e1", EDITOR, "t"),
            ("term", TERMINAL, "t"),
            ("e2", EDITOR, "t"),
            ("other", EDITOR, "u"),
        ]);
        let flags = DirtyPanes::default();
        flags.mark(PaneId::new("e1").unwrap());
        flags.mark(PaneId::new("e2").unwrap());
        let incoming = [open("e3", EDITOR, "t")];
        let gone = evictions(&layout, &incoming, 3, &flags);
        assert!(gone.is_empty(), "nothing is left to close: {gone:?}");
    }

    #[test]
    fn opening_a_chat_or_focusing_a_pane_evicts_nothing() {
        let layout = layout_of(&[("c", CHAT, "t"), ("e1", EDITOR, "t")]);
        let flags = DirtyPanes::default();
        let chat = open("c2", CHAT, "t");
        let focus = Command::FocusPane {
            pane: PaneId::new("e1").unwrap(),
        };
        let gone = evictions(&layout, &[chat, focus], 1, &flags);
        assert!(gone.is_empty(), "nothing is evicted: {gone:?}");
    }
}
