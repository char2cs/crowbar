use super::*;

#[test]
fn summary_pluralises_and_skips_zero_counts() {
    let cases = [
        (2, 1, "2 uncommitted files · 1 unmerged commit"),
        (1, 0, "1 uncommitted file"),
        (0, 3, "3 unmerged commits"),
        (0, 0, ""),
    ];
    for (files, commits, expected) in cases {
        let risk = WorkAtRisk {
            branch: "b".into(),
            uncommitted_files: files,
            unmerged_commits: commits,
        };
        assert_eq!(risk.summary(), expected, "{files} files {commits} commits");
    }
}
