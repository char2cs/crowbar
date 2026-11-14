use super::*;
use crate::control::Control;
use crate::value::Value;

#[test]
fn ten_pages_in_rail_order_with_unique_keys() {
    let schema = default_schema().unwrap();
    let ids: Vec<&str> = schema
        .pages()
        .iter()
        .map(|page| page.id().as_str())
        .collect();
    assert_eq!(
        ids,
        [
            "appearance",
            "editor",
            "file-explorer",
            "git",
            "terminal",
            "browser",
            "providers",
            "active-sessions",
            "keybindings",
            "hosts",
        ]
    );
}

#[test]
fn every_default_matches_the_shape_its_control_stores() {
    let schema = default_schema().unwrap();
    for row in schema.rows() {
        let Some(default) = &row.default else {
            continue;
        };
        let fits = match (&row.control, default) {
            (Control::Number { range, .. }, Value::Number(n)) => {
                (range.min..=range.max).contains(n)
            },
            (Control::Switch { .. }, Value::Bool(_))
            | (Control::Text { .. } | Control::Select { .. }, Value::Text(_))
            | (Control::TextList { .. }, Value::List(_)) => true,
            _ => false,
        };
        assert!(
            fits,
            "{}: {default:?} does not fit {:?}",
            row.key(),
            row.control
        );
    }
}

#[test]
fn fixed_select_defaults_are_among_the_options() {
    let schema = default_schema().unwrap();
    for row in schema.rows() {
        let (
            Control::Select {
                source: crate::ChoiceSource::Fixed(choices),
                ..
            },
            Some(Value::Text(default)),
        ) = (&row.control, &row.default)
        else {
            continue;
        };
        assert!(
            choices.iter().any(|choice| choice.value() == default),
            "{}: {default}",
            row.key()
        );
    }
}

#[test]
fn every_page_icon_is_a_known_phosphor_stem() {
    let schema = default_schema().unwrap();
    let stems = [
        "paint-brush",
        "code-block",
        "tree-structure",
        "git-branch",
        "terminal-window",
        "globe",
        "robot",
        "pulse",
        "keyboard",
        "hard-drives",
    ];
    for page in schema.pages() {
        assert!(stems.contains(&page.icon.as_str()), "{}", page.icon);
    }
}
