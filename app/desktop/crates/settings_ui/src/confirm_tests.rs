use super::*;

#[test]
fn keeps_the_three_texts() {
    let confirm = Confirm::new("Clear?", "All of it.", "Clear data");
    assert_eq!(
        (
            confirm.title.as_str(),
            confirm.message.as_str(),
            confirm.action.as_str()
        ),
        ("Clear?", "All of it.", "Clear data")
    );
}
