use model::{Command, Layout, PaneInstance, Placement, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use crate::error::Result;
use crate::serials::Serials;

pub(crate) fn open_commands(
    layout: &Layout,
    pane: (PaneTypeId, Params),
    serials: &Serials,
) -> Result<Vec<Command>> {
    let (type_id, params) = pane;
    if let Some(existing) = layout.find_pane(&type_id, &params) {
        return Ok(focus_commands(layout, &existing.pane_id));
    }
    let instance = PaneInstance {
        pane_id: PaneId::new(serials.next_pane())?,
        type_id,
        params,
        state: PaneState::default(),
    };
    let Some(tab) = layout.active_tab() else {
        return Ok(open_in_new_tab(serials.next_tab(), instance));
    };
    Ok(vec![open_pane(tab.id(), instance)])
}

pub(crate) fn open_at(
    pane: (PaneTypeId, Params),
    (tab, placement): (TabId, Placement),
    serials: &Serials,
) -> Result<Command> {
    let (type_id, params) = pane;
    let instance = PaneInstance {
        pane_id: PaneId::new(serials.next_pane())?,
        type_id,
        params,
        state: PaneState::default(),
    };
    Ok(Command::OpenPane {
        tab,
        instance,
        placement: Some(placement),
    })
}

fn open_pane(
    tab: TabId,
    instance: PaneInstance,
) -> Command {
    Command::OpenPane {
        tab,
        instance,
        placement: None,
    }
}

fn open_in_new_tab(
    id: TabId,
    instance: PaneInstance,
) -> Vec<Command> {
    let add = Command::AddTab {
        id,
        title: String::new(),
    };
    vec![add, open_pane(id, instance)]
}

fn focus_commands(
    layout: &Layout,
    pane: &PaneId,
) -> Vec<Command> {
    let tab = layout.tab_of(pane).map(model::Tab::id);
    let mut commands: Vec<Command> = tab
        .map(|tab| Command::ActivateTab { tab })
        .into_iter()
        .collect();
    commands.push(Command::FocusPane { pane: pane.clone() });
    commands
}

#[cfg(test)]
mod tests {
    use model::{Edge, TabId};

    use super::*;

    fn chat() -> PaneTypeId {
        PaneTypeId::new("core.chat").unwrap()
    }

    fn thread(id: &str) -> Params {
        Params::default().with("threadId", id).unwrap()
    }

    fn apply(
        layout: &mut Layout,
        commands: Vec<Command>,
    ) {
        for command in commands {
            layout.apply(command).unwrap();
        }
    }

    fn open_all(threads: &[&str]) -> Layout {
        let mut layout = Layout::default();
        let serials = Serials::above(&layout);
        for name in threads {
            let commands = open_commands(&layout, (chat(), thread(name)), &serials).unwrap();
            apply(&mut layout, commands);
        }
        layout
    }

    type Outcome = (usize, usize, Option<&'static str>);

    type Case = (&'static str, &'static [&'static str], Outcome);

    const CASES: &[Case] = &[
        (
            "the first pane opens in a new tab",
            &["a"],
            (1, 1, Some("p1")),
        ),
        (
            "later panes open beside the focused one",
            &["a", "b"],
            (1, 2, Some("p2")),
        ),
        (
            "an open pane is focused instead of opened twice",
            &["a", "b", "a"],
            (1, 2, Some("p1")),
        ),
    ];

    #[test]
    fn opening_a_pane_reuses_what_is_already_open() {
        for (name, threads, (tabs, panes, focused)) in CASES {
            let layout = open_all(threads);
            let focus = layout.active_tab().and_then(|tab| tab.focused().cloned());
            assert_eq!(
                (layout.tabs().len(), layout.pane_count()),
                (*tabs, *panes),
                "{name}"
            );
            assert_eq!(
                focus.map(|id| id.to_string()),
                focused.map(str::to_owned),
                "{name}: focus"
            );
            assert_eq!(
                layout.active_tab().map(model::Tab::id),
                Some(TabId(1)),
                "{name}: tab"
            );
        }
    }

    fn dropped(
        layout: &mut Layout,
        thread_name: &str,
        (target, edge): (&str, Edge),
    ) {
        let placement = Placement {
            target: PaneId::new(target).unwrap(),
            edge,
        };
        let serials = Serials::above(layout);
        let command = open_at(
            (chat(), thread(thread_name)),
            (TabId(1), placement),
            &serials,
        )
        .unwrap();
        apply(layout, vec![command]);
    }

    #[test]
    fn a_chat_dropped_on_a_pane_opens_beside_it_on_that_side() {
        let mut layout = open_all(&["a"]);
        dropped(&mut layout, "b", ("p1", Edge::Left));
        let tab = layout.active_tab().unwrap();
        assert_eq!(layout.pane_count(), 2, "two panes");
        let names: Vec<_> = tab.pane_ids().iter().map(ToString::to_string).collect();
        assert_eq!(names, ["p2", "p1"], "the new chat is left of the first");
    }

    #[test]
    fn a_chat_that_is_open_is_moved_by_a_drop_never_duplicated() {
        let mut layout = open_all(&["a", "b"]);
        dropped(&mut layout, "a", ("p2", Edge::Right));
        assert_eq!(layout.pane_count(), 2, "still two panes");
        let tab = layout.active_tab().unwrap();
        let names: Vec<_> = tab.pane_ids().iter().map(ToString::to_string).collect();
        assert_eq!(names, ["p2", "p1"], "a moved to the right of b");
    }
}
