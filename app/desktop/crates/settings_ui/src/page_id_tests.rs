use super::*;

#[test]
fn round_trips_its_text() {
    let id = PageId::from("keybindings");
    assert_eq!(
        (id.as_str(), id.to_string()),
        ("keybindings", "keybindings".to_owned())
    );
}
