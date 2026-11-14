use super::*;

#[test]
fn gitignore_is_inverted_and_filters_default_to_empty_lists() {
    let page = page();
    let gitignore = page
        .rows()
        .find(|row| row.key().as_str() == "showGitignoredFilesInFileTree");
    assert!(matches!(
        gitignore.map(|row| &row.control),
        Some(Control::Switch { invert: true })
    ));
    let filters: Vec<&Row> = page
        .rows()
        .filter(|row| row.key().as_str().starts_with("hidden"))
        .collect();
    assert_eq!(filters.len(), 2);
    assert!(
        filters
            .iter()
            .all(|row| row.default == Some(Value::List(Vec::new())))
    );
}
