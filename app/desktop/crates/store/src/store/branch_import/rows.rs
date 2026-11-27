use crate::store::branch_import::availability::Availability;
use crate::store::branch_import::entry::Entry;
use crate::store::branch_import::import_branch::ImportBranch;

fn availability_rank(branch: &Entry) -> bool {
    branch.availability() != Availability::Protected
}

pub(crate) fn of(list: &[Entry]) -> Vec<ImportBranch> {
    let (free, protected): (Vec<_>, Vec<_>) =
        list.iter().partition(|branch| availability_rank(branch));
    protected
        .into_iter()
        .chain(free)
        .map(|branch| ImportBranch {
            name: branch.name.clone(),
            availability: branch.availability(),
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn branch(json: &str) -> Entry {
        serde_json::from_str(json).unwrap()
    }

    #[test]
    fn protected_branches_come_first_and_each_keeps_its_availability() {
        let listed = of(&[
            branch(r#"{"name":"free"}"#),
            branch(r#"{"name":"main","isProtected":true}"#),
            branch(r#"{"name":"done","hasWorkspace":true}"#),
        ]);
        let names: Vec<&str> = listed.iter().map(|row| row.name.as_str()).collect();
        assert_eq!(names, ["main", "free", "done"]);
        assert_eq!(listed[0].availability, Availability::Protected);
    }
}
