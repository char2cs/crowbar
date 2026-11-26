//! Where a node sits among its siblings, as the daemon's `compareNodes` sorts them: order, then
//! kind (folder, branch, chat, repo), then creation time, then id.

use crate::rows::forest::{Item, ItemKind};

const FRACTION_DIGITS: usize = 9;
const FOLDER_RANK: u8 = 0;
const BRANCH_RANK: u8 = 1;
const CHAT_RANK: u8 = 2;
const REPO_RANK: u8 = 3;

#[derive(Debug, Clone, Default, PartialEq, Eq, PartialOrd, Ord)]
pub(crate) struct SiblingKey {
    order: i64,
    rank: u8,
    created: String,
    id: String,
}

impl SiblingKey {
    pub(crate) fn of_item(item: &Item) -> Self {
        Self {
            order: item.order,
            rank: rank_of(item.kind),
            created: created_key(&item.created),
            id: item.id.clone(),
        }
    }

    pub(crate) fn of_repo(
        order: i64,
        id: &str,
    ) -> Self {
        Self {
            order,
            rank: REPO_RANK,
            created: created_key(""),
            id: id.to_owned(),
        }
    }
}

fn created_key(stamp: &str) -> String {
    let (head, rest) = stamp.split_once('.').unwrap_or((stamp, ""));
    let head = head.trim_end_matches('Z');
    let digits: String = rest.chars().take_while(char::is_ascii_digit).collect();
    format!("{head}.{digits:0<FRACTION_DIGITS$}")
}

fn rank_of(kind: ItemKind) -> u8 {
    match kind {
        ItemKind::Folder => FOLDER_RANK,
        ItemKind::Workspace => BRANCH_RANK,
        ItemKind::Chat => CHAT_RANK,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn item(
        id: &str,
        kind: ItemKind,
        order: i64,
    ) -> Item {
        Item {
            id: id.into(),
            kind,
            order,
            created: String::new(),
            filed: String::new(),
            ground: String::new(),
        }
    }

    #[test]
    fn order_comes_first_then_kind_with_repos_last_then_id() {
        let chat = SiblingKey::of_item(&item("a", ItemKind::Chat, 0));
        let folder = SiblingKey::of_item(&item("z", ItemKind::Folder, 0));
        let repo = SiblingKey::of_repo(0, "b");
        let later = SiblingKey::of_item(&item("c", ItemKind::Folder, 1));
        let tie = SiblingKey::of_item(&item("b", ItemKind::Chat, 0));
        let mut keys = vec![
            later.clone(),
            repo.clone(),
            tie.clone(),
            chat.clone(),
            folder.clone(),
        ];
        keys.sort();
        assert_eq!(keys, [folder, chat, tie, repo, later]);
    }

    #[test]
    fn stamps_of_different_precision_compare_by_instant() {
        let earlier = created_key("2026-11-15T02:13:33.05Z");
        let later = created_key("2026-11-15T02:13:33.050047789Z");
        assert!(earlier < later);
        assert_eq!(
            created_key("2026-11-15T02:13:33Z"),
            created_key("2026-11-15T02:13:33.0Z")
        );
        assert_eq!(created_key(""), ".000000000");
    }
}
