//! The listings a folder browser holds, each applied only by the latest request for its path.

use std::collections::HashMap;

use crate::shell_data::folder_browse::folder_listing::FolderListing;

#[derive(Debug, Default)]
pub(crate) struct FolderHeld {
    listings: HashMap<String, FolderListing>,
    latest: HashMap<String, u64>,
    issued: u64,
}

impl FolderHeld {
    pub(crate) fn listing(
        &self,
        path: &str,
    ) -> Option<FolderListing> {
        self.listings.get(path).cloned()
    }

    pub(crate) fn is_listed(
        &self,
        path: &str,
    ) -> bool {
        self.listings.contains_key(path)
    }

    pub(crate) fn begin(
        &mut self,
        path: &str,
    ) -> u64 {
        self.issued += 1;
        self.latest.insert(path.to_owned(), self.issued);
        self.listings
            .insert(path.to_owned(), FolderListing::Loading);
        self.issued
    }

    pub(crate) fn settle(
        &mut self,
        path: &str,
        request: u64,
        listing: FolderListing,
    ) -> bool {
        if self.latest.get(path) != Some(&request) {
            return false;
        }
        self.latest.remove(path);
        self.listings.insert(path.to_owned(), listing);
        true
    }

    pub(crate) fn is_failed(
        &self,
        path: &str,
    ) -> bool {
        matches!(self.listings.get(path), Some(FolderListing::Failed(_)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_latest_request_for_a_path_settles_it() {
        let mut held = FolderHeld::default();
        let first = held.begin("/a");
        let second = held.begin("/a");
        assert!(!held.settle("/a", first, FolderListing::Failed("old".into())));
        assert_eq!(held.listing("/a"), Some(FolderListing::Loading));
        assert!(held.settle("/a", second, FolderListing::Loaded(Vec::new())));
        assert_eq!(held.listing("/a"), Some(FolderListing::Loaded(Vec::new())));
        assert!(
            !held.settle("/a", second, FolderListing::Loading),
            "settled once"
        );
    }

    #[test]
    fn a_path_is_listed_once_asked_and_failed_only_after_a_refusal() {
        let mut held = FolderHeld::default();
        assert!(!held.is_listed("/a"));
        let request = held.begin("/a");
        assert!(held.is_listed("/a") && !held.is_failed("/a"));
        held.settle("/a", request, FolderListing::Failed("no".into()));
        assert!(held.is_failed("/a"));
    }
}
