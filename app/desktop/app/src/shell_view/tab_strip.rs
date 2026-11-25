use gpui::Context;
use layout::Tab;
use protocol::PaneId;
use settings::{Key, Read as _};
use sidebar::{Activity, Row, Titling};
use viewtabs::{MemberGlyph, MemberKind, MemberView, TabView};

use super::ShellView;
use crate::chat_label::{NEW_CHAT, unloaded_label};
use crate::tab_scope::TabScope;

pub(super) const TAB_LIST_SCOPE: &str = "tabListScope";
const TERMINAL_LABEL: &str = "Terminal";
const THREAD_ID: &str = "threadId";

impl ShellView {
    pub(super) fn push_tabs(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let scope = self.tab_scope();
        let space = self.state.space();
        let titles = self.state.titles();
        let layout = self.state.layout();
        let rows = |chat: &str| self.data.chat_row(chat);
        let tabs: Vec<TabView> = layout
            .tabs()
            .iter()
            .filter(|tab| scope.lists(&space, tab.space()))
            .map(|tab| TabView {
                id: tab.id(),
                members: crate::group_focus::listed(tab)
                    .iter()
                    .map(|pane| member((&titles, layout), &rows, pane))
                    .collect(),
            })
            .collect();
        let active = layout.showing(&space).map(Tab::id);
        self.tabs
            .update(cx, |bar, cx| bar.set_tabs(tabs, active, cx));
    }

    fn tab_scope(&mut self) -> TabScope {
        let stored = self
            .settings_backend()
            .and_then(|backend| backend.get(&Key::new(TAB_LIST_SCOPE)));
        TabScope::from_setting(stored.as_ref().and_then(settings::Value::as_text))
    }
}

type ChatRows<'a> = &'a dyn Fn(&str) -> Option<Row>;

fn member(
    (titles, layout): (&crate::titles::Titles, &layout::Layout),
    chat_rows: ChatRows<'_>,
    pane: &PaneId,
) -> MemberView {
    let kind = kind_of(layout, pane);
    let thread = thread_of(layout, pane);
    let row = thread.and_then(chat_rows);
    if kind == MemberKind::Chat {
        return chat_member(pane, (thread, row));
    }
    let working = row.as_ref().is_some_and(is_working);
    let title = titles.get(pane);
    MemberView {
        pane: pane.clone(),
        label: title
            .as_ref()
            .map_or_else(|| untitled(kind).into(), |title| title.to_string().into()),
        provisional: title.is_none(),
        working,
        kind,
        glyph: None,
    }
}

fn is_working(row: &Row) -> bool {
    row.activity == Activity::Working
}

fn chat_member(
    pane: &PaneId,
    (thread, row): (Option<&str>, Option<Row>),
) -> MemberView {
    let working = row.as_ref().is_some_and(is_working);
    let glyph = row.as_ref().map(sidebar_glyph);
    let unloaded = unloaded_label(thread);
    let (label, provisional) = row.map_or((unloaded.into(), true), |row| {
        (row.label, row.titling == Titling::Provisional)
    });
    MemberView {
        pane: pane.clone(),
        label: label.into(),
        provisional,
        working,
        kind: MemberKind::Chat,
        glyph,
    }
}

fn sidebar_glyph(row: &Row) -> MemberGlyph {
    let glyph = sidebarview::ChatGlyph::of(row);
    MemberGlyph {
        icon: glyph.icon,
        weight: glyph.weight,
        color: glyph.tone.color(),
    }
}

fn thread_of<'a>(
    layout: &'a layout::Layout,
    pane: &PaneId,
) -> Option<&'a str> {
    layout.pane(pane)?.params.get(THREAD_ID)
}

fn kind_of(
    layout: &layout::Layout,
    pane: &PaneId,
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chat_label::{LOADING_CHAT, NEW_THREAD};
    use crate::core_panes::{CHAT, NEW_TAB, TERMINAL};
    use crate::titles::Titles;

    fn layout_holding(
        pane: &PaneId,
        type_id: &str,
        params: protocol::Params,
    ) -> layout::Layout {
        let mut layout = layout::Layout::default();
        let tab = layout::TabId(1);
        layout
            .apply(layout::Command::AddTab {
                id: tab,
                title: "t".into(),
                space: layout::SpaceId::default(),
            })
            .unwrap();
        let instance = layout::PaneInstance {
            pane_id: pane.clone(),
            type_id: protocol::PaneTypeId::new(type_id).unwrap(),
            params,
            state: protocol::PaneState::default(),
        };
        layout
            .apply(layout::Command::OpenPane {
                tab,
                instance,
                placement: None,
            })
            .unwrap();
        layout
    }

    fn titled(
        pane: &PaneId,
        title: &str,
    ) -> Titles {
        let titles = Titles::default();
        titles.set(pane.clone(), protocol::PaneTitle::new(title).unwrap());
        titles
    }

    fn thread(chat: &str) -> protocol::Params {
        protocol::Params::default().with(THREAD_ID, chat).unwrap()
    }

    fn row(
        label: &str,
        titling: Titling,
        activity: Activity,
    ) -> Row {
        let mut row = Row::new("c1", sidebar::RowKind::Chat, 0, label);
        row.titling = titling;
        row.activity = activity;
        row
    }

    #[test]
    fn a_chat_member_reads_its_title_from_the_chat_not_from_its_page() {
        type Case = (Option<(&'static str, Titling)>, (&'static str, bool));
        const CASES: [Case; 3] = [
            (
                Some(("Perf fixture", Titling::Final)),
                ("Perf fixture", false),
            ),
            (
                Some(("Untitled chat", Titling::Provisional)),
                ("Untitled chat", true),
            ),
            (None, (LOADING_CHAT, true)),
        ];
        let pane = PaneId::new("p7").unwrap();
        let layout = layout_holding(&pane, CHAT, thread("c1"));
        let titles = titled(&pane, "New chat");
        for (known, (label, provisional)) in CASES {
            let rows = |_: &str| known.map(|(text, titling)| row(text, titling, Activity::Idle));
            let member = member((&titles, &layout), &rows, &pane);
            assert_eq!(
                (member.label.as_ref(), member.provisional, member.kind),
                (label, provisional, MemberKind::Chat),
                "{known:?}"
            );
        }
    }

    #[test]
    fn a_chat_member_leads_with_its_sidebar_glyph() {
        let pane = PaneId::new("p7").unwrap();
        let layout = layout_holding(&pane, CHAT, thread("c1"));
        let titles = Titles::default();
        let locked = |_: &str| {
            let mut held = row("main", Titling::Final, Activity::Idle);
            held.lock = sidebar::Lock::Locked;
            Some(held)
        };
        let glyph = member((&titles, &layout), &locked, &pane).glyph;
        assert_eq!(
            glyph.map(|glyph| glyph.icon),
            Some(ui::IconName::Lock),
            "a locked workspace's chat shows the lock, as develop's view tab does"
        );
    }

    #[test]
    fn a_chat_member_spins_while_its_chat_works() {
        let pane = PaneId::new("p7").unwrap();
        let layout = layout_holding(&pane, CHAT, thread("c1"));
        let titles = Titles::default();
        let busy = |_: &str| Some(row("Review", Titling::Final, Activity::Working));
        assert!(member((&titles, &layout), &busy, &pane).working, "working");
        let idle = |_: &str| Some(row("Review", Titling::Final, Activity::Idle));
        assert!(!member((&titles, &layout), &idle, &pane).working, "idle");
    }

    #[test]
    fn other_members_carry_their_page_title_or_the_kind_default() {
        type Case = (
            &'static str,
            Option<&'static str>,
            (MemberKind, &'static str, bool),
        );
        const CASES: [Case; 4] = [
            (TERMINAL, Some("zsh"), (MemberKind::Terminal, "zsh", false)),
            (TERMINAL, None, (MemberKind::Terminal, "Terminal", true)),
            (
                NEW_TAB,
                Some("New Tab"),
                (MemberKind::NewTab, "New Tab", false),
            ),
            (NEW_TAB, None, (MemberKind::NewTab, NEW_CHAT, true)),
        ];
        let none = |_: &str| None;
        for (type_id, title, (kind, label, provisional)) in CASES {
            let pane = PaneId::new("p7").unwrap();
            let titles = title.map_or_else(Titles::default, |text| titled(&pane, text));
            let layout = layout_holding(&pane, type_id, protocol::Params::default());
            let member = member((&titles, &layout), &none, &pane);
            assert_eq!(
                (member.label.as_ref(), member.kind, member.provisional),
                (label, kind, provisional),
                "{type_id} {title:?}"
            );
        }
    }

    #[test]
    fn a_draft_reads_new_thread_provisionally() {
        let pane = PaneId::new("p7").unwrap();
        let params = thread("draft-u1").with("projectId", "p2").unwrap();
        let layout = layout_holding(&pane, CHAT, params);
        let none = |_: &str| None;
        let member = member((&Titles::default(), &layout), &none, &pane);
        assert_eq!(
            (member.label.as_ref(), member.provisional),
            (NEW_THREAD, true)
        );
    }
}
