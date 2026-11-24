use layout::{Command, Layout, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};
use viewtabs::MemberView;

pub(crate) type DemoResult<T> = Result<T, Box<dyn std::error::Error>>;

const CHIP_NAMES: [&str; 2] = ["delta", "epsilon"];
const CHAT_TYPE: &str = "core.chat";

fn instance(name: &str) -> DemoResult<PaneInstance> {
    Ok(PaneInstance {
        pane_id: PaneId::new(name)?,
        type_id: PaneTypeId::new(CHAT_TYPE)?,
        params: Params::default().with("chat", name)?,
        state: PaneState::default(),
    })
}

pub(crate) fn chips() -> DemoResult<Vec<PaneInstance>> {
    CHIP_NAMES.into_iter().map(instance).collect()
}

pub(crate) fn member(pane: &PaneId) -> MemberView {
    let title = match pane.as_str() {
        "p1" => "Untitled chat",
        "p2" => "Fix the sidebar drag",
        "p3" => "Release notes",
        "p4" => "Review the migration plan",
        other => other,
    };
    MemberView {
        pane: pane.clone(),
        label: title.to_owned().into(),
        provisional: pane.as_str() == "p1",
        working: pane.as_str() == "p3",
        kind: viewtabs::MemberKind::Chat,
        glyph: None,
    }
}

const MANY_TABS: u64 = 14;
const DEFAULT_TABS: u64 = 3;

fn add_chat_tab(
    layout: &mut Layout,
    index: u64,
) -> DemoResult<()> {
    let title = format!("p{index}");
    let tab = TabId(index);
    layout.apply(Command::AddTab {
        id: tab,
        title: title.clone(),
        space: layout::SpaceId::default(),
    })?;
    layout.apply(Command::OpenPane {
        tab,
        instance: instance(&title)?,
        placement: None,
    })?;
    Ok(())
}

pub(crate) fn build(name: &str) -> DemoResult<Layout> {
    let count: u64 = match name {
        "one" => 1,
        "many" => MANY_TABS,
        _ => DEFAULT_TABS,
    };
    let mut layout = Layout::default();
    (1..=count).try_for_each(|index| add_chat_tab(&mut layout, index))?;
    layout.apply(Command::ActivateTab { tab: TabId(1) })?;
    Ok(layout)
}
