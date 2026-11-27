use crate::shell::{Row, RowKind};

#[derive(Debug, Clone, Copy)]
pub struct RowTree<'a> {
    rows: &'a [Row],
}

impl<'a> RowTree<'a> {
    pub fn new(rows: &'a [Row]) -> Self {
        Self { rows }
    }

    pub fn parent(
        self,
        index: usize,
    ) -> Option<usize> {
        let depth = self.rows.get(index)?.depth;
        self.rows
            .iter()
            .take(index)
            .rposition(|row| row.depth < depth)
    }

    pub fn children(
        self,
        container: Option<usize>,
    ) -> impl Iterator<Item = usize> + use<'a> {
        (0..self.rows.len()).filter(move |index| self.parent(*index) == container)
    }

    pub fn anchor(
        self,
        from: Option<usize>,
    ) -> Option<usize> {
        self.chain(from)
            .take_while(|(_, row)| row.kind != RowKind::RepoHeader)
            .find(|(_, row)| row.kind == RowKind::Branch)
            .map(|(index, _)| index)
    }

    pub fn repo(
        self,
        from: Option<usize>,
    ) -> Option<usize> {
        self.chain(from)
            .find(|(_, row)| row.kind == RowKind::RepoHeader)
            .map(|(index, _)| index)
    }

    fn chain(
        self,
        from: Option<usize>,
    ) -> impl Iterator<Item = (usize, &'a Row)> {
        std::iter::successors(from, move |index| self.parent(*index))
            .filter_map(move |index| self.rows.get(index).map(|row| (index, row)))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rows() -> Vec<Row> {
        vec![
            Row::new("home", RowKind::Folder, 0, "notes"),
            Row::new("repo", RowKind::RepoHeader, 1, "demo"),
            Row::new("branch", RowKind::Branch, 2, "feature"),
            Row::new("folder", RowKind::Folder, 3, "src"),
            Row::new("chat", RowKind::Chat, 2, "loose"),
            Row::new("after", RowKind::Chat, 0, "home chat"),
        ]
    }

    #[test]
    fn a_row_knows_its_parent_branch_and_repository_by_depth() {
        let rows = rows();
        let tree = RowTree::new(&rows);
        assert_eq!(tree.parent(3), Some(2), "parent");
        assert_eq!(tree.anchor(Some(3)), Some(2), "anchor through a folder");
        assert_eq!(tree.anchor(Some(4)), None, "the checkout");
        assert_eq!(
            tree.repo(Some(3)),
            Some(1),
            "a repository inside a home folder"
        );
        assert_eq!(tree.repo(Some(5)), None, "a home row after it");
        assert_eq!(tree.children(None).collect::<Vec<_>>(), [0, 5], "top level");
        assert_eq!(
            tree.children(Some(1)).collect::<Vec<_>>(),
            [2, 4],
            "repository"
        );
    }
}
