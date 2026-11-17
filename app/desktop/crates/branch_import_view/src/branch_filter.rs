//! Which branches the list shows and what it says when it shows none.

use shell_data::{BranchListing, ImportBranch};

use crate::row_look::{FAILED, LOADING, NONE_MATCH, NONE_ON_REMOTE};

pub(crate) fn matching(
    listing: &BranchListing,
    filter: &str,
) -> Vec<ImportBranch> {
    let BranchListing::Ready(rows) = listing else {
        return Vec::new();
    };
    let query = filter.to_lowercase();
    rows.iter()
        .filter(|row| row.name.to_lowercase().contains(&query))
        .cloned()
        .collect()
}

pub(crate) fn empty_message(
    listing: &BranchListing,
    shown: usize,
) -> Option<&'static str> {
    match listing {
        BranchListing::Loading => Some(LOADING),
        BranchListing::Failed => Some(FAILED),
        BranchListing::Ready(rows) if rows.is_empty() => Some(NONE_ON_REMOTE),
        BranchListing::Ready(_) if shown == 0 => Some(NONE_MATCH),
        BranchListing::Ready(_) => None,
    }
}

#[cfg(test)]
mod tests {
    use shell_data::Availability;

    use super::*;

    fn branch(
        name: &str,
        availability: Availability,
    ) -> ImportBranch {
        ImportBranch {
            name: name.to_owned(),
            availability,
        }
    }

    type Message = (&'static str, usize, Option<&'static str>);

    const MESSAGES: [Message; 4] = [
        ("loading", 0, Some(LOADING)),
        ("failed", 0, Some(FAILED)),
        ("no filter match", 0, Some(NONE_MATCH)),
        ("rows shown", 2, None),
    ];

    #[test]
    fn the_list_says_why_it_is_empty() {
        let ready = BranchListing::Ready(vec![
            branch("a", Availability::Importable),
            branch("b", Availability::Importable),
        ]);
        let listings = [
            BranchListing::Loading,
            BranchListing::Failed,
            ready.clone(),
            ready,
        ];
        for ((name, shown, expected), listing) in MESSAGES.into_iter().zip(&listings) {
            assert_eq!(empty_message(listing, shown), expected, "{name}");
        }
        assert_eq!(
            empty_message(&BranchListing::Ready(Vec::new()), 0),
            Some(NONE_ON_REMOTE)
        );
    }

    #[test]
    fn the_filter_keeps_the_branches_whose_name_contains_it_in_any_case() {
        let listing = BranchListing::Ready(vec![
            branch("Feature-A", Availability::Importable),
            branch("main", Availability::Protected),
        ]);
        let names: Vec<_> = matching(&listing, "feat")
            .into_iter()
            .map(|row| row.name)
            .collect();
        assert_eq!(names, ["Feature-A"]);
        assert_eq!(matching(&BranchListing::Loading, "").len(), 0);
    }
}
