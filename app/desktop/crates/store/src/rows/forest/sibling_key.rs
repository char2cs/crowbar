use crate::rows::forest::created_key::created_key;
use crate::rows::forest::{Item, ItemKind};

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
}

fn rank_of(kind: ItemKind) -> u8 {
    match kind {
        ItemKind::Folder => FOLDER_RANK,
        ItemKind::Workspace => BRANCH_RANK,
        ItemKind::Chat => CHAT_RANK,
        ItemKind::Repo => REPO_RANK,
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
        let repo = SiblingKey::of_item(&item("b", ItemKind::Repo, 0));
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
}
