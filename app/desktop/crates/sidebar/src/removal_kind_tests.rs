use super::*;

#[test]
fn only_repositories_and_spaces_ask_first() {
    let cases = [
        (RemovalKind::Project, "space", true),
        (RemovalKind::Repo, "repository", true),
        (RemovalKind::Workspace, "workspace", false),
        (RemovalKind::Chat, "chat", false),
        (RemovalKind::Folder, "folder", false),
    ];
    for (kind, noun, confirms) in cases {
        assert_eq!(kind.noun(), noun, "{kind:?} noun");
        assert_eq!(kind.needs_confirmation(), confirms, "{kind:?} confirmation");
    }
}
