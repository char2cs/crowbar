use super::*;

#[test]
fn reads_the_scripts_report() {
    assert_eq!(parse_hover(r#"{"sdk":1,"kind":"shell.hover","hovered":true}"#), Some(true), "enter");
    assert_eq!(parse_hover(r#"{"sdk":1,"kind":"shell.hover","hovered":false}"#), Some(false), "leave");
}

#[test]
fn everything_else_is_not_a_hover() {
    assert_eq!(parse_hover(r#"{"sdk":1,"kind":"pane.setTitle","title":"x"}"#), None, "other frame");
    assert_eq!(parse_hover("not json"), None, "garbage");
}
