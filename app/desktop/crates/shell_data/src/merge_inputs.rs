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

    struct Case<'a> {
        added: u32,
        uncommitted: usize,
        status: &'a str,
        open_threads: u32,
        can_merge: bool,
        has_parent: bool,
    }

    fn reason(case: &Case<'_>) -> Option<String> {
        let workspace = WorkspaceRecord {
            added: case.added,
            status: case.status.to_owned(),
            can_merge_locally: case.can_merge,
            ..WorkspaceRecord::default()
        };
        let inputs = MergeInputs {
            parent: case.has_parent.then_some("main"),
            uncommitted: case.uncommitted,
            open_threads: case.open_threads,
            workspace: &workspace,
        };
        inputs.blocked()
    }

    const MERGEABLE: Case<'static> = Case {
        added: 3,
        uncommitted: 0,
        status: "",
        open_threads: 0,
        can_merge: true,
        has_parent: true,
    };

    #[test]
    fn a_branch_with_nothing_or_unsettled_work_cannot_merge() {
        let cases = [
            (
                Case {
                    added: 0,
                    ..MERGEABLE
                },
                "Nothing to merge yet — this branch has no changes.",
            ),
            (
                Case {
                    uncommitted: 2,
                    ..MERGEABLE
                },
                "2 uncommitted changes — commit them first.",
            ),
            (
                Case {
                    status: "pr-conflicts",
                    ..MERGEABLE
                },
                "Conflicts with main — resolve them first.",
            ),
        ];
        for (case, expected) in cases {
            assert_eq!(reason(&case).as_deref(), Some(expected));
        }
        assert_eq!(reason(&MERGEABLE), None);
    }

    #[test]
    fn a_protected_orphaned_or_discussed_branch_cannot_merge() {
        let cases = [
            (
                Case {
                    can_merge: false,
                    ..MERGEABLE
                },
                "main is protected — open a pull request instead.",
            ),
            (
                Case {
                    has_parent: false,
                    ..MERGEABLE
                },
                "This branch has no parent to merge into.",
            ),
            (
                Case {
                    open_threads: 1,
                    ..MERGEABLE
                },
                "1 open thread must be resolved before merging.",
            ),
        ];
        for (case, expected) in cases {
            assert_eq!(reason(&case).as_deref(), Some(expected));
        }
        assert_eq!(reason(&MERGEABLE), None);
    }
}
