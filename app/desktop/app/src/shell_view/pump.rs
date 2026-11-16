use gpui::Context;
use gpui::Window;
use tab_bar::{MemberKind, MemberView};

use super::ShellView;
use crate::chat_label::NEW_CHAT;

const TERMINAL_LABEL: &str = "Terminal";

pub(super) fn member(
    (titles, layout): (&crate::titles::Titles, &model::Layout),
    pane: &protocol::PaneId,
) -> MemberView {
    let title = titles.get(pane);
    let kind = kind_of(layout, pane);
    MemberView {
        pane: pane.clone(),
        label: title
            .as_ref()
            .map_or_else(|| untitled(kind).into(), |title| title.to_string().into()),
        provisional: title.is_none(),
        working: false,
        kind,
    }
}

fn kind_of(
    layout: &model::Layout,
    pane: &protocol::PaneId,
) -> MemberKind {
    match layout.pane(pane).map(|instance| instance.type_id.as_str()) {
        Some(crate::core_panes::TERMINAL) => MemberKind::Terminal,
        Some(crate::core_panes::NEW_TAB) => MemberKind::NewTab,
        _ => MemberKind::Chat,
    }
}

fn untitled(kind: MemberKind) -> &'static str {
    if kind == MemberKind::Terminal {
        return TERMINAL_LABEL;
    }
    NEW_CHAT
}

type Step<T> = fn(&mut ShellView, T, &mut Window, &mut Context<'_, ShellView>);

pub(super) async fn drain<T>(
    source: async_channel::Receiver<T>,
    this: gpui::WeakEntity<ShellView>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<T>,
) {
    while stepped(&source, &this, cx, step).await {}
}

async fn stepped<T>(
    source: &async_channel::Receiver<T>,
    this: &gpui::WeakEntity<ShellView>,
    cx: &mut gpui::AsyncWindowContext,
    step: Step<T>,
) -> bool {
    let Ok(item) = source.recv().await else {
        return false;
    };
    this.update_in(cx, |view, window, cx| step(view, item, window, cx))
        .is_ok()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::titles::Titles;

    #[test]
    fn a_member_is_idle_and_carries_its_pane_and_title() {
        let titles = Titles::default();
        let pane = protocol::PaneId::new("p7").unwrap();
        titles.set(pane.clone(), protocol::PaneTitle::new("Review").unwrap());
        let member = member((&titles, &model::Layout::default()), &pane);
        assert_eq!(member.pane, pane, "pane");
        assert_eq!(member.label.as_ref(), "Review", "title");
        assert!(!member.working, "idle");
    }

    #[test]
    fn a_terminal_member_is_named_and_drawn_as_a_terminal() {
        let mut layout = model::Layout::default();
        let tab = model::TabId(1);
        layout
            .apply(model::Command::AddTab {
                id: tab,
                title: "t".into(),
            })
            .unwrap();
        let pane = protocol::PaneId::new("t1").unwrap();
        let instance = model::PaneInstance {
            pane_id: pane.clone(),
            type_id: protocol::PaneTypeId::new(crate::core_panes::TERMINAL).unwrap(),
            params: protocol::Params::default(),
            state: protocol::PaneState::default(),
        };
        layout
            .apply(model::Command::OpenPane {
                tab,
                instance,
                placement: None,
            })
            .unwrap();
        let member = member((&Titles::default(), &layout), &pane);
        assert_eq!(member.kind, MemberKind::Terminal, "glyph");
        assert_eq!(member.label.as_ref(), "Terminal", "label");
    }
}
