use super::*;

#[test]
fn the_font_row_explains_nerd_fonts_and_all_eight_consumers_have_rows() {
    let page = page();
    assert_eq!(page.rows().count(), 8);
    let font = page.rows().next().map(|row| &row.control);
    assert!(matches!(
        font,
        Some(Control::Select {
            hint: Some(_),
            placeholder: Some(_),
            ..
        })
    ));
}
