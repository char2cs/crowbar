use super::*;

#[test]
fn the_custom_url_row_waits_for_the_custom_engine_and_clear_data_has_a_capability() {
    let page = page();
    let custom = page
        .rows()
        .find(|row| row.key().as_str() == "browserCustomSearchUrl");
    assert!(custom.is_some_and(|row| row.when.is_some() && row.invalid_description.is_some()));
    let clear = page
        .rows()
        .find(|row| row.key().as_str() == "browserClearData");
    let Some(Control::Button(spec)) = clear.map(|row| &row.control) else {
        panic!("the privacy row holds a button");
    };
    assert_eq!(
        spec.enabled,
        Some(Condition::capability("browser.clearData"))
    );
    assert!(spec.confirm.is_some() && spec.disabled_note.is_some());
}
