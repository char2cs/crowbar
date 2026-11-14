use super::*;
use crate::control::Control;

#[test]
fn headerless_keeps_the_title_and_rows_keep_their_order() {
    let section = Section::new("Theme")
        .headerless()
        .describe("Colours")
        .row(Row::new("a", "A", Control::switch()))
        .row(Row::new("b", "B", Control::switch()));
    assert!(!section.show_header);
    assert_eq!(section.title, "Theme");
    assert_eq!(section.description.as_deref(), Some("Colours"));
    let keys: Vec<&str> = section.rows.iter().map(|row| row.key().as_str()).collect();
    assert_eq!(keys, ["a", "b"]);
}
