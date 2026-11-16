use model::{Command, PaneInstance, Tab, TabId};
use protocol::{PaneId, PaneTypeId};

use crate::core_panes::CHAT;

pub(crate) fn stacks(type_id: &PaneTypeId) -> bool {
    type_id.as_str() != CHAT
}

pub(crate) fn stack_target(tab: &Tab) -> Option<PaneId> {
    let stackable = |pane: &PaneId| tab.pane(pane).is_some_and(|found| stacks(&found.type_id));
    let focused = tab.focused().filter(|pane| stackable(pane));
    focused
        .cloned()
        .or_else(|| tab.pane_ids().into_iter().find(stackable))
}

pub(crate) fn listed(tab: &Tab) -> Vec<PaneId> {
    let chats: Vec<PaneId> = tab
        .pane_ids()
        .into_iter()
        .filter(|pane| tab.pane(pane).is_some_and(|found| !stacks(&found.type_id)))
        .collect();
    if !chats.is_empty() {
        return chats;
    }
    tab.pane_ids()
        .first()
        .and_then(|first| tab.group_of(first))
        .map(|group| group.active().clone())
        .into_iter()
        .collect()
}

pub(crate) fn place(
    tab: &Tab,
    instance: PaneInstance,
) -> Command {
    let onto = stacks(&instance.type_id)
        .then(|| stack_target(tab))
        .flatten();
    match onto {
        Some(onto) => Command::OpenPaneInGroup {
            instance,
            onto,
            index: None,
        },
        None => split(tab.id(), instance),
    }
}

fn split(
    tab: TabId,
    instance: PaneInstance,
) -> Command {
    Command::OpenPane {
        tab,
        instance,
        placement: None,
    }
}

#[cfg(test)]
mod tests {
    use model::Layout;
    use protocol::{PaneState, Params};

    use super::*;

    fn instance(
        name: &str,
        kind: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: PaneTypeId::new(kind).unwrap(),
            params: Params::default().with("id", name).unwrap(),
            state: PaneState::default(),
        }
    }

    fn layout_of(panes: &[(&str, &str)]) -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: String::new(),
            })
            .unwrap();
        for (name, kind) in panes {
            let tab = layout.active_tab().unwrap();
            let command = place(tab, instance(name, kind));
            layout.apply(command).unwrap();
        }
        layout
    }

    fn strips(layout: &Layout) -> Vec<Vec<String>> {
        let tab = layout.active_tab().unwrap();
        let bounds = model::Rect {
            x: 0.0,
            y: 0.0,
            width: 4000.0,
            height: 1000.0,
        };
        tab.groups(bounds)
            .iter()
            .map(|group| group.panes.iter().map(ToString::to_string).collect())
            .collect()
    }

    struct Case {
        name: &'static str,
        opened: &'static [(&'static str, &'static str)],
        strips: &'static [&'static [&'static str]],
    }

    const CASES: &[Case] = &[
        Case {
            name: "a terminal beside a chat opens a strip of one",
            opened: &[("a", "core.chat"), ("t1", "core.terminal")],
            strips: &[&["a"], &["t1"]],
        },
        Case {
            name: "more terminals stack in that strip",
            opened: &[
                ("a", "core.chat"),
                ("t1", "core.terminal"),
                ("t2", "core.terminal"),
                ("f1", "core.editor"),
            ],
            strips: &[&["a"], &["t1", "t2", "f1"]],
        },
        Case {
            name: "chats only ever split",
            opened: &[("a", "core.chat"), ("b", "core.chat"), ("c", "core.chat")],
            strips: &[&["a"], &["b"], &["c"]],
        },
        Case {
            name: "a stackable pane alone in a tab starts the first strip",
            opened: &[("t1", "core.terminal"), ("t2", "core.browser")],
            strips: &[&["t1", "t2"]],
        },
        Case {
            name: "the focused chat does not take a terminal into its leaf",
            opened: &[
                ("t1", "core.terminal"),
                ("a", "core.chat"),
                ("t2", "core.terminal"),
            ],
            strips: &[&["t1", "t2"], &["a"]],
        },
    ];

    #[test]
    fn stackable_panes_join_a_strip_and_chats_split() {
        for case in CASES {
            let layout = layout_of(case.opened);
            let expected: Vec<Vec<String>> = case
                .strips
                .iter()
                .map(|strip| strip.iter().map(|name| (*name).to_owned()).collect())
                .collect();
            assert_eq!(strips(&layout), expected, "{}", case.name);
        }
    }

    struct Listing {
        name: &'static str,
        opened: &'static [(&'static str, &'static str)],
        listed: &'static [&'static str],
    }

    const LISTINGS: &[Listing] = &[
        Listing {
            name: "chats are the view tabs, strips are not",
            opened: &[
                ("a", "core.chat"),
                ("b", "core.chat"),
                ("t1", "core.terminal"),
                ("t2", "core.terminal"),
            ],
            listed: &["a", "b"],
        },
        Listing {
            name: "a tab of one strip lists the shown pane of it",
            opened: &[("t1", "core.terminal"), ("t2", "core.editor")],
            listed: &["t2"],
        },
    ];

    #[test]
    fn the_top_bar_lists_chats_and_only_falls_back_to_a_strip_when_there_is_none() {
        for case in LISTINGS {
            let layout = layout_of(case.opened);
            let tab = layout.active_tab().unwrap();
            let names: Vec<String> = listed(tab).iter().map(ToString::to_string).collect();
            assert_eq!(names, case.listed, "{}", case.name);
        }
    }

    #[test]
    fn only_a_chat_refuses_to_stack() {
        for (kind, expected) in [
            ("core.chat", false),
            ("core.terminal", true),
            ("core.editor", true),
            ("core.review", true),
            ("core.newtab", true),
            ("ext.acme.widget", true),
        ] {
            assert_eq!(stacks(&PaneTypeId::new(kind).unwrap()), expected, "{kind}");
        }
    }
}
