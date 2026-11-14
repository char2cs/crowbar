use super::*;

fn number() -> Control {
    Control::number(
        NumberRange::new(8.0, 32.0, 1.0),
        ControlWidth::NumberCompact,
    )
}

#[test]
fn text_round_trips_through_the_value() {
    let list = Control::text_list("*.log", ControlWidth::Text);
    let value = Value::List(vec!["a".to_owned(), "b/".to_owned()]);
    assert_eq!(list.text_of(Some(&value)), "a, b/");
    assert_eq!(list.parse(" a , ,b/ ,"), Ok(value));
    assert_eq!(number().text_of(Some(&Value::Number(14.0))), "14");
    assert_eq!(number().text_of(None), "");
}

#[test]
fn numbers_are_normalised_when_parsed() {
    assert_eq!(number().parse("99"), Ok(Value::Number(32.0)));
    assert_eq!(number().parse("x"), Err(Issue::NotANumber));
    assert_eq!(number().issue("x"), Some(Issue::NotANumber));
    assert_eq!(number().issue("12"), None);
}

#[test]
fn text_is_kept_as_typed_but_flagged() {
    let url = Control::text("https://kagi.com/search?q=%s", ControlWidth::TextWide)
        .validated(Validator::SearchTemplate);
    assert_eq!(url.parse("nope"), Ok(Value::Text("nope".to_owned())));
    assert_eq!(url.issue("nope"), Some(Issue::BadSearchTemplate));
    assert_eq!(url.issue(""), None);
}

#[test]
fn modifiers_apply_only_to_their_control() {
    let select = Control::provided_select("themes", ControlWidth::Wide)
        .placeholder("Select font...")
        .hint("Install a Nerd Font");
    assert!(matches!(
        select,
        Control::Select {
            placeholder: Some(_),
            hint: Some(_),
            ..
        }
    ));
    assert_eq!(
        Control::switch().placeholder("x").hint("y"),
        Control::switch()
    );
}
