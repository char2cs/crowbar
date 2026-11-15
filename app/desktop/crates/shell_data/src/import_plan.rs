//! What an import will do, as the dialog's hint states it.

use std::collections::{HashMap, HashSet};

use crate::availability::Availability;
use crate::branch_entry::BranchEntry;
use crate::pr_link::PrLink;

/// How many branches come in and how many missing parents the daemon will create for them.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct ImportPlan {
    /// Branches selected.
    pub import_count: usize,
    /// Ancestors along the open-PR chains that do not yet exist as workspaces.
    pub parent_count: usize,
}

fn terminals(branches: &[BranchEntry]) -> HashSet<&str> {
    branches
        .iter()
        .filter(|branch| branch.availability() != Availability::Importable)
        .map(|branch| branch.name.as_str())
        .collect()
}

type Chains<'a> = (
    &'a HashMap<&'a str, &'a str>,
    &'a HashSet<&'a str>,
    &'a HashSet<&'a str>,
);

fn created_parents<'a>(
    branch: &'a str,
    (base, stop, selected): Chains<'a>,
    default_branch: &str,
) -> Vec<&'a str> {
    let mut visited = HashSet::from([branch]);
    std::iter::successors(base.get(branch).copied(), |parent| {
        base.get(parent).copied()
    })
    .take_while(|parent| *parent != default_branch && visited.insert(parent))
    .take_while(|parent| !stop.contains(parent))
    .filter(|parent| !selected.contains(parent))
    .collect()
}

pub(crate) fn of(
    selected: &[String],
    links: &[PrLink],
    (branches, default_branch): (&[BranchEntry], &str),
) -> ImportPlan {
    let base: HashMap<&str, &str> = links
        .iter()
        .filter(|link| !link.head.is_empty() && !link.base.is_empty())
        .map(|link| (link.head.as_str(), link.base.as_str()))
        .collect();
    let stop = terminals(branches);
    let chosen: HashSet<&str> = selected.iter().map(String::as_str).collect();
    let parents: HashSet<&str> = selected
        .iter()
        .flat_map(|branch| created_parents(branch, (&base, &stop, &chosen), default_branch))
        .collect();
    ImportPlan {
        import_count: selected.len(),
        parent_count: parents.len(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn branch(
        name: &str,
        extra: &str,
    ) -> BranchEntry {
        serde_json::from_str(&format!(r#"{{"name":"{name}"{extra}}}"#)).unwrap()
    }

    fn link(
        head: &str,
        base: &str,
    ) -> PrLink {
        PrLink {
            head: head.into(),
            base: base.into(),
        }
    }

    type Case = (
        &'static str,
        &'static [&'static str],
        &'static [(&'static str, &'static str)],
        usize,
    );

    const CASES: [Case; 5] = [
        ("no pull requests", &["a"], &[], 0),
        (
            "a parent that is not selected is created",
            &["b"],
            &[("b", "a"), ("a", "main")],
            1,
        ),
        (
            "a selected parent is not created",
            &["a", "b"],
            &[("b", "a"), ("a", "main")],
            0,
        ),
        (
            "an imported parent ends the walk",
            &["c"],
            &[("c", "x"), ("x", "i"), ("i", "main")],
            1,
        ),
        (
            "a protected parent ends the walk",
            &["b"],
            &[("b", "p"), ("p", "main")],
            0,
        ),
    ];

    #[test]
    fn missing_parents_along_the_open_pr_chain_are_counted_once() {
        let branches = [
            branch("i", r#","hasWorkspace":true"#),
            branch("p", r#","isProtected":true"#),
        ];
        for (name, selected, links, parents) in CASES {
            let selected: Vec<String> = selected.iter().map(|s| (*s).to_owned()).collect();
            let links: Vec<PrLink> = links.iter().map(|(h, b)| link(h, b)).collect();
            let plan = of(&selected, &links, (&branches, "main"));
            assert_eq!(
                (plan.import_count, plan.parent_count),
                (selected.len(), parents),
                "{name}"
            );
        }
    }
}
