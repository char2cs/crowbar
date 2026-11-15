use layout_view::PaneSource as _;
use protocol::PaneTitle;

use super::*;

#[test]
fn an_unnamed_chat_is_called_new_chat_and_a_named_one_by_its_title() {
    let titles = Titles::default();
    let source = ShellPaneSource::new(titles.clone(), Serials::above(&model::Layout::default()));
    let pane = PaneId::new("p1").unwrap();
    assert_eq!(source.label(&pane).as_ref(), "New chat", "unnamed");
    titles.set(pane.clone(), PaneTitle::new("Fix").unwrap());
    assert_eq!(source.label(&pane).as_ref(), "Fix", "named");
}

#[test]
fn detached_panes_get_fresh_tabs() {
    let source = ShellPaneSource::new(Titles::default(), Serials::above(&model::Layout::default()));
    let pane = PaneId::new("p1").unwrap();
    let (first, _) = source.fresh_tab(&pane);
    let (second, _) = source.fresh_tab(&pane);
    assert_ne!(first, second, "never the same id twice");
}
