use super::*;

#[test]
fn two_switches_in_two_sections() {
    let page = page();
    let keys: Vec<&str> = page.rows().map(|row| row.key().as_str()).collect();
    assert_eq!(keys, ["compactGitStatusBadges", "showGitStatusInFileTree"]);
    assert_eq!(page.blocks.len(), 2);
}
