use super::*;

#[test]
fn rows_follow_the_spec_order_and_only_the_first_section_hides_its_header() {
    let page = page();
    let keys: Vec<&str> = page.rows().map(|row| row.key().as_str()).collect();
    assert_eq!(
        keys,
        [
            "theme",
            "themeMode",
            "iconTheme",
            "uiFontFamily",
            "headingFontFamily",
            "uiFontSize",
            "sidebarPosition",
            "tabListScope",
            "sidebarFollowsFocus",
            "buildReadout",
        ]
    );
    let headers: Vec<bool> = page
        .blocks
        .iter()
        .filter_map(|block| match block {
            crate::block::Block::Section(section) => Some(section.show_header),
            _ => None,
        })
        .collect();
    assert_eq!(headers, [false, true, true, true]);
}
