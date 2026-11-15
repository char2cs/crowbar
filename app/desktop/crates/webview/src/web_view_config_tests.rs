use super::*;

#[test]
fn debug_shows_the_page_and_hides_the_script_and_handler() {
    let config = WebViewConfig {
        url: "crowbar://local/x".into(),
        bounds: Bounds {
            x: 0.0,
            y: 0.0,
            width: 1.0,
            height: 1.0,
        },
        kind: WebViewKind::Core,
        visibility: Visibility::Hidden,
        initialization_script: Some("secret-script".into()),
        on_ipc: Arc::new(|_| {}),
    };
    let text = format!("{config:?}");
    assert!(text.contains("crowbar://local/x"), "url shown: {text}");
    assert!(!text.contains("secret-script"), "script hidden: {text}");
}
