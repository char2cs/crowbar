use std::sync::Arc;

use super::*;
use crate::bounds::Bounds;
use crate::call::Call;
use crate::mock_platform::MockPlatform;
use crate::parent_window::ParentWindow;
use crate::web_view_config::WebViewConfig;
use crate::web_view_kind::WebViewKind;

fn handle() -> (Rc<MockPlatform>, WebViewHandle) {
    let mock = Rc::new(MockPlatform::default());
    let bounds = Bounds {
        x: 0.0,
        y: 0.0,
        width: 1.0,
        height: 1.0,
    };
    let config = WebViewConfig {
        url: "x".into(),
        bounds,
        kind: WebViewKind::Core,
        visibility: Visibility::Shown,
        initialization_script: None,
        on_ipc: Arc::new(|_| {}),
    };
    mock.create(WebViewId(0), ParentWindow::X11(1), config)
        .unwrap();
    (Rc::clone(&mock), WebViewHandle::new(mock, WebViewId(0)))
}

#[test]
fn show_and_hide_only_change_visibility() {
    let (mock, view) = handle();
    view.hide().unwrap();
    view.show().unwrap();
    let visibilities: Vec<Visibility> = mock
        .calls()
        .iter()
        .filter_map(|call| match call {
            Call::SetVisibility { visibility, .. } => Some(*visibility),
            _ => None,
        })
        .collect();
    assert_eq!(
        visibilities,
        [Visibility::Hidden, Visibility::Shown],
        "hide then show"
    );
    assert_eq!(
        mock.calls().len(),
        3,
        "create plus two visibility changes, nothing else"
    );
}

#[test]
fn post_message_escapes_the_text_into_a_script_literal() {
    let (mock, view) = handle();
    view.post_message("a\"b\n</script>").unwrap();
    let script = mock.calls().into_iter().find_map(|call| match call {
        Call::Eval { script, .. } => Some(script),
        _ => None,
    });
    assert_eq!(
        script.as_deref(),
        Some(r#"window.__crowbarReceive&&window.__crowbarReceive("a\"b\n</script>")"#),
        "script"
    );
}

#[test]
fn dropping_destroys_exactly_once() {
    let (mock, view) = handle();
    drop(view);
    assert_eq!(
        mock.live_views(),
        Vec::<WebViewId>::new(),
        "destroyed on drop"
    );
    let (mock, view) = handle();
    view.destroy().unwrap();
    let destroys = mock
        .calls()
        .iter()
        .filter(|c| matches!(c, Call::Destroy { .. }))
        .count();
    assert_eq!(destroys, 1, "explicit destroy is not repeated by drop");
}
