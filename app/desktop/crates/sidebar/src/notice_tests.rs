use super::*;

#[test]
fn the_text_names_the_repository() {
    assert_eq!(
        Notice::Project.text(),
        "Couldn't load this project's chats",
        "project"
    );
    assert_eq!(
        Notice::Repo("demo".into()).text(),
        "Couldn't load chats in demo",
        "repo"
    );
}
