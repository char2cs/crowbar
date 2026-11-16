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
    use crate::core_panes::{CHAT, NEW_TAB, TERMINAL};
    use crate::titles::Titles;

    type Case = (
        &'static str,
        Option<&'static str>,
        (MemberKind, &'static str, bool),
    );

    const CASES: [Case; 5] = [
        (CHAT, Some("Review"), (MemberKind::Chat, "Review", false)),
        (TERMINAL, Some("zsh"), (MemberKind::Terminal, "zsh", false)),
        (CHAT, None, (MemberKind::Chat, NEW_CHAT, true)),
        (TERMINAL, None, (MemberKind::Terminal, "Terminal", true)),
        (NEW_TAB, None, (MemberKind::NewTab, NEW_CHAT, true)),
    ];

    fn layout_holding(
        pane: &protocol::PaneId,
        type_id: &str,
    ) -> model::Layout {
        let mut layout = model::Layout::default();
        let tab = model::TabId(1);
        layout
            .apply(model::Command::AddTab {
                id: tab,
                title: "t".into(),
            })
            .unwrap();
        let instance = model::PaneInstance {
            pane_id: pane.clone(),
            type_id: protocol::PaneTypeId::new(type_id).unwrap(),
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
        layout
    }

    fn titled(
        pane: &protocol::PaneId,
        title: Option<&str>,
    ) -> Titles {
        let titles = Titles::default();
        if let Some(text) = title {
            titles.set(pane.clone(), protocol::PaneTitle::new(text).unwrap());
        }
        titles
    }

    #[test]
    fn a_member_carries_its_pane_and_a_title_or_the_kind_default_until_one_arrives() {
        for (type_id, title, (kind, label, provisional)) in CASES {
            let pane = protocol::PaneId::new("p7").unwrap();
            let titles = titled(&pane, title);
            let member = member((&titles, &layout_holding(&pane, type_id)), &pane);
            assert_eq!(
                (member.pane, member.label.as_ref(), member.kind),
                (pane, label, kind),
                "{type_id} {title:?}"
            );
            assert_eq!(member.provisional, provisional, "{type_id} {title:?}");
            assert!(!member.working, "{type_id} {title:?}");
        }
    }
}
