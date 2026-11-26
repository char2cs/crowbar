//! Everything the chats panel of one space draws.

use crate::shell::chats::notice::Notice;
use crate::shell::chats::reach::Reach;
use crate::shell::chats::row::Row;
use crate::shell::chats::row_id::RowId;

/// The rows of a space, a load failure to show, and whether its host answers.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct ProjectTree {
    /// Rows in tree order.
    pub rows: Vec<Row>,
    /// A failed load to report above the rows.
    pub notice: Option<Notice>,
    /// Reachability of the space's host.
    pub reach: Reach,
}

impl ProjectTree {
    /// `id` and every row drawn beneath it, in tree order; empty when `id` is not drawn.
    #[must_use]
    pub fn subtree(
        &self,
        id: &RowId,
    ) -> Vec<RowId> {
        let from = self.rows.iter().position(|row| &row.id == id);
        let Some(below) = from.and_then(|at| self.rows.get(at..)) else {
            return Vec::new();
        };
        let Some(top) = below.first() else {
            return Vec::new();
        };
        let beneath = below
            .iter()
            .skip(1)
            .take_while(|lower| lower.depth > top.depth);
        std::iter::once(top)
            .chain(beneath)
            .map(|row| row.id.clone())
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::chats::row_kind::RowKind;

    fn tree() -> ProjectTree {
        let rows = [("b", 1), ("t", 2), ("n", 3), ("s", 1)]
            .into_iter()
            .map(|(id, depth)| Row::new(id, RowKind::Chat, depth, id))
            .collect();
        ProjectTree {
            rows,
            ..ProjectTree::default()
        }
    }

    #[test]
    fn a_subtree_is_the_row_and_the_deeper_rows_up_to_its_next_sibling() {
        let ids = |id: &str| tree().subtree(&RowId::from(id));
        assert_eq!(ids("b"), [RowId::from("b"), "t".into(), "n".into()]);
        assert_eq!(ids("n"), [RowId::from("n")], "a leaf");
        assert_eq!(ids("s"), [RowId::from("s")], "the last row");
        assert!(ids("gone").is_empty(), "not drawn");
    }
}
