use gpui::TestAppContext;

use crate::intent::Intent;
use crate::key::Key;
use crate::settings_backend::SettingsBackend;
use crate::test_support::{inspect, open, row, run};
use crate::value::Value;

#[gpui::test]
fn a_toggle_reaches_the_backend_and_the_row_becomes_resettable(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let target = row("sidebarFollowsFocus");
    run(&harness, Intent::Toggle(target.clone()), cx);
    assert_eq!(
        harness.backend.get(&Key::new("sidebarFollowsFocus")),
        Some(Value::Bool(true))
    );
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .resettable(&target)));
}

#[gpui::test]
fn a_refused_write_goes_back_and_says_so(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let target = row("sidebarFollowsFocus");
    harness.backend.refuse_writes();
    run(&harness, Intent::Toggle(target.clone()), cx);
    assert_eq!(harness.backend.get(&target.key), None);
    assert!(!inspect(&harness, cx, |modal| modal
        .state
        .switch_on(&target)));
    assert!(inspect(&harness, cx, |modal| modal.notice.is_some()));
    run(&harness, Intent::Select("editor".into()), cx);
    assert!(inspect(&harness, cx, |modal| modal.notice.is_none()));
}

#[gpui::test]
fn a_change_made_elsewhere_shows_up(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    harness
        .backend
        .publish(&Key::new("sidebarFollowsFocus"), Value::Bool(true));
    cx.run_until_parked();
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .switch_on(&row("sidebarFollowsFocus"))));
}

#[gpui::test]
fn closing_calls_the_close_handler_once(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    run(&harness, Intent::Close, cx);
    assert_eq!(harness.closed.get(), 1);
}

#[gpui::test]
fn every_page_renders_with_and_without_a_search(cx: &mut TestAppContext) {
    let (harness, cx) = open(cx, "appearance");
    let pages: Vec<_> = inspect(&harness, cx, |modal| {
        modal
            .state
            .visible_pages()
            .iter()
            .map(|page| page.id.clone())
            .collect()
    });
    assert_eq!(pages.len(), 10);
    for page in pages {
        run(&harness, Intent::Select(page.clone()), cx);
        assert_eq!(
            inspect(&harness, cx, |modal| modal
                .state
                .page()
                .map(|page| page.id.clone())),
            Some(page)
        );
    }
    let typed = Intent::Search(ui::TextInputEvent::Changed("zzzzzz".to_owned()));
    run(&harness, typed, cx);
    assert!(inspect(&harness, cx, |modal| modal
        .state
        .visible_pages()
        .is_empty()));
}

#[gpui::test]
fn the_query_option_fills_the_search_field(cx: &mut TestAppContext) {
    cx.update(|cx| ui::init(cx).unwrap());
    let backend = crate::test_support::backend();
    let config = crate::modal_config::ModalConfig::new(
        crate::catalog::default_schema().unwrap(),
        std::rc::Rc::new(backend.clone()),
        std::rc::Rc::new(backend),
    );
    let (modal, cx) = cx.add_window_view(|window, cx| {
        crate::settings_modal::SettingsModal::new(config, window, cx).with_query(Some("font"), cx)
    });
    cx.run_until_parked();
    assert_eq!(
        modal.read_with(cx, |modal, _| modal.state.query().to_owned()),
        "font"
    );
}
