use protocol::{PaneState, PaneTypeId, Params};

use crate::command::Command;
use crate::edge::Edge;
use crate::layout::Layout;
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::tab_id::TabId;
use crate::test_support::tab_fixtures::id;

pub(crate) fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: id(name),
        type_id: PaneTypeId::new("core.terminal").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

pub(crate) fn chat(
    name: &str,
    chat_id: &str,
) -> PaneInstance {
    PaneInstance {
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default().with("chatId", chat_id).unwrap(),
        ..instance(name)
    }
}

pub(crate) fn open(
    tab: u64,
    instance: PaneInstance,
) -> Command {
    Command::OpenPane {
        tab: TabId(tab),
        instance,
        placement: None,
    }
}

pub(crate) fn add(tab: u64) -> Command {
    Command::AddTab {
        id: TabId(tab),
        title: format!("tab {tab}"),
    }
}

pub(crate) fn layout(commands: Vec<Command>) -> Layout {
    let mut layout = Layout::default();
    for command in commands {
        layout.apply(command).unwrap();
    }
    layout.validate().unwrap();
    layout
}

pub(crate) fn move_onto_itself(name: &str) -> Command {
    let placement = Placement {
        target: id(name),
        edge: Edge::Left,
    };
    Command::MovePane {
        pane: id(name),
        placement,
    }
}

pub(crate) fn sample() -> Layout {
    layout(vec![
        add(1),
        open(1, chat("a", "c1")),
        open(1, instance("b")),
        add(2),
        open(2, instance("c")),
    ])
}

/// One tab (id 1) whose first group stacks `names` in order, the last one shown and focused.
pub(crate) fn stacked(names: &[&str]) -> Layout {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(1),
            title: "t".into(),
        })
        .unwrap();
    layout.apply(open(1, instance(names[0]))).unwrap();
    names[1..]
        .iter()
        .map(|name| stacked_in(name, names[0]))
        .for_each(|command| layout.apply(command).unwrap());
    layout
}

fn stacked_in(
    name: &str,
    onto: &str,
) -> Command {
    Command::OpenPaneInGroup {
        instance: instance(name),
        onto: id(onto),
        index: None,
    }
}

/// The first group of tab 1 written as its members in order, the pinned ones in brackets.
pub(crate) fn strip(layout: &Layout) -> String {
    let tab = layout.tab(TabId(1)).unwrap();
    let group = tab.group_of(&tab.pane_ids()[0]).unwrap();
    let written: Vec<String> = group
        .panes()
        .iter()
        .map(|pane| {
            let pinned = group.is_pinned(pane).then(|| format!("[{pane}]"));
            pinned.unwrap_or_else(|| pane.to_string())
        })
        .collect();
    written.join(" ")
}
