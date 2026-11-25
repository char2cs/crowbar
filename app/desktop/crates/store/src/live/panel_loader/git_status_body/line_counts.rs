//! Added and deleted lines per file, summed over the staged and the unstaged diff.

use std::collections::HashMap;

use crate::live::panel_loader::git_status_body::git_diff_body::GitDiffBody;

pub(crate) type LineCounts = HashMap<String, (u32, u32)>;

pub(crate) fn of(diffs: &[GitDiffBody]) -> LineCounts {
    let mut counts = LineCounts::new();
    for diff in diffs {
        let held = counts.entry(diff.file_path.clone()).or_default();
        *held = (
            held.0.saturating_add(diff.additions),
            held.1.saturating_add(diff.deletions),
        );
    }
    counts
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_file_in_both_diffs_sums_its_lines() {
        let diffs: Vec<GitDiffBody> = serde_json::from_str(
            r#"[{"file_path":"a","additions":2,"deletions":1},{"file_path":"a","additions":3},{"file_path":"b"}]"#,
        )
        .unwrap();
        let counts = of(&diffs);
        assert_eq!(counts["a"], (5, 1));
        assert_eq!(counts["b"], (0, 0));
    }
}
