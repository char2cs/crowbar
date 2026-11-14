use super::*;
use crate::project_id::ProjectId;
use crate::row_id::RowId;

fn repo(at_risk: Vec<WorkAtRisk>) -> Confirmation {
    Confirmation {
        target: RemovalTarget::Row(RowId::from("r")),
        kind: RemovalKind::Repo,
        label: "demo".into(),
        at_risk,
    }
}

#[test]
fn first_question_names_the_noun_and_the_cascade() {
    let confirmation = repo(Vec::new());
    assert_eq!(confirmation.title(), "Delete repository “demo”?", "title");
    assert_eq!(confirmation.confirm_label(), "Delete repository", "button");
    assert!(
        confirmation.lead().0.starts_with("All workspaces"),
        "repo cascade"
    );
}

#[test]
fn work_at_risk_changes_every_line() {
    let risk = WorkAtRisk {
        branch: "b".into(),
        uncommitted_files: 1,
        unmerged_commits: 0,
    };
    let confirmation = repo(vec![risk]);
    assert_eq!(
        confirmation.title(),
        "Delete repository “demo” and lose work?",
        "title"
    );
    assert_eq!(confirmation.confirm_label(), "Delete anyway", "button");
    assert!(
        confirmation
            .lead()
            .0
            .starts_with("This work exists nowhere else"),
        "lead"
    );
}

#[test]
fn spaces_use_the_space_cascade() {
    let mut confirmation = repo(Vec::new());
    confirmation.kind = RemovalKind::Project;
    confirmation.target = RemovalTarget::Project(ProjectId::from("p"));
    assert!(
        confirmation.lead().0.starts_with("All repositories"),
        "space cascade"
    );
    assert_eq!(confirmation.title(), "Delete space “demo”?", "space title");
}
