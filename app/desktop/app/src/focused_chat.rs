use model::Layout;

pub(crate) fn focused_chat(layout: &Layout) -> Option<String> {
    let tab = layout.active_tab()?;
    let pane = layout.pane(tab.focused()?)?;
    pane.params.get("threadId").map(str::to_owned)
}

#[cfg(test)]
mod tests {
    use model::{Command, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, Params};

    use super::*;
    use crate::core_panes::{chat_params, chat_type};

    fn open(
        layout: &mut Layout,
        params: Params,
    ) {
        let instance = PaneInstance {
            pane_id: PaneId::new("p1").unwrap(),
            type_id: chat_type().unwrap(),
            params,
            state: PaneState::default(),
        };
        let tab = TabId(1);
        let title = String::new();
        layout.apply(Command::AddTab { id: tab, title }).unwrap();
        let placement = None;
        layout
            .apply(Command::OpenPane {
                tab,
                instance,
                placement,
            })
            .unwrap();
    }

    #[test]
    fn the_focused_pane_names_the_chat() {
        let mut layout = Layout::default();
        assert_eq!(focused_chat(&layout), None, "nothing open");
        open(&mut layout, chat_params("c7").unwrap());
        assert_eq!(focused_chat(&layout).as_deref(), Some("c7"));
    }

    #[test]
    fn a_pane_without_a_chat_names_none() {
        let mut layout = Layout::default();
        open(&mut layout, Params::default());
        assert_eq!(focused_chat(&layout), None);
    }
}
