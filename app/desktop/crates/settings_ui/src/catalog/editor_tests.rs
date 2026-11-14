use super::*;

#[test]
fn every_editor_setting_of_the_spec_has_a_row_with_a_default() {
    let page = page();
    assert_eq!(page.rows().count(), 18);
    assert!(
        page.rows().all(|row| row.default.is_some()),
        "every row is resettable"
    );
    let first = page.rows().next().map(|row| row.key().as_str());
    assert_eq!(first, Some("fontFamily"));
    assert_eq!(
        page.rows().last().map(|row| row.key().as_str()),
        Some("markdownFontSize")
    );
}
