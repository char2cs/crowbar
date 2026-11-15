//! What decides whether the followed workspace can merge.

use crate::workspace_record::WorkspaceRecord;

const CONFLICTS: &str = "pr-conflicts";

pub(crate) struct MergeInputs<'a> {
    pub(crate) parent: Option<&'a str>,
    pub(crate) uncommitted: usize,
    pub(crate) open_threads: u32,
    pub(crate) workspace: &'a WorkspaceRecord,
}

fn plural(
    count: usize,
    noun: &str,
) -> String {
    let suffix = match count {
        1 => "",
        _ => "s",
    };
    format!("{count} {noun}{suffix}")
}

impl MergeInputs<'_> {
    fn change_reason(&self) -> Option<String> {
        let workspace = self.workspace;
        let unchanged = workspace.added == 0 && workspace.deleted == 0 && self.uncommitted == 0;
        if unchanged {
            return Some("Nothing to merge yet — this branch has no changes.".to_owned());
        }
        (self.uncommitted > 0).then(|| {
            format!(
                "{} — commit them first.",
                plural(self.uncommitted, "uncommitted change")
            )
        })
    }

    fn parent_reason(&self) -> Option<String> {
        let Some(parent) = self.parent else {
            return Some("This branch has no parent to merge into.".to_owned());
        };
        if self.workspace.status == CONFLICTS {
            return Some(format!("Conflicts with {parent} — resolve them first."));
        }
        (!self.workspace.can_merge_locally)
            .then(|| format!("{parent} is protected — open a pull request instead."))
    }

    fn thread_reason(&self) -> Option<String> {
        (self.open_threads > 0).then(|| {
            format!(
                "{} must be resolved before merging.",
                plural(self.open_threads as usize, "open thread")
            )
        })
    }

    pub(crate) fn blocked(&self) -> Option<String> {
        self.change_reason()
            .or_else(|| self.parent_reason())
            .or_else(|| self.thread_reason())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn reason(
        (added, uncommitted, status): (u32, usize, &str),
        (open_threads, can_merge, has_parent): (u32, bool, bool),
    ) -> Option<String> {
        let workspace = WorkspaceRecord {
            added,
            status: status.to_owned(),
            can_merge_locally: can_merge,
            ..WorkspaceRecord::default()
        };
        let inputs = MergeInputs {
            parent: has_parent.then_some("main"),
            uncommitted,
            open_threads,
            workspace: &workspace,
        };
        inputs.blocked()
    }

    #[test]
    fn the_first_applicable_reason_blocks_the_merge() {
        let cases = [
            (
                reason((0, 0, ""), (0, true, true)),
                "Nothing to merge yet — this branch has no changes.",
            ),
            (
                reason((3, 2, ""), (0, true, true)),
                "2 uncommitted changes — commit them first.",
            ),
            (
                reason((3, 0, "pr-conflicts"), (0, true, true)),
                "Conflicts with main — resolve them first.",
            ),
            (
                reason((3, 0, ""), (0, false, true)),
                "main is protected — open a pull request instead.",
            ),
            (
                reason((3, 0, ""), (0, true, false)),
                "This branch has no parent to merge into.",
            ),
            (
                reason((3, 0, ""), (1, true, true)),
                "1 open thread must be resolved before merging.",
            ),
        ];
        for (found, expected) in cases {
            assert_eq!(found.as_deref(), Some(expected));
        }
        assert_eq!(reason((3, 0, ""), (0, true, true)), None);
    }
}
