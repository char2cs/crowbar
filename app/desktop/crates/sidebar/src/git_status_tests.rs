use super::*;

#[test]
fn letters_follow_the_status() {
    let cases = [
        (GitStatus::Clean, None),
        (GitStatus::Ignored, None),
        (GitStatus::Added, Some('A')),
        (GitStatus::Modified, Some('M')),
        (GitStatus::Deleted, Some('D')),
        (GitStatus::Untracked, Some('U')),
    ];
    for (status, letter) in cases {
        assert_eq!(status.letter(), letter, "{status:?}");
    }
}
