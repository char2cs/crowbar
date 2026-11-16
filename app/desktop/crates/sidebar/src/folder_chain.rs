//! Single-child folder chains that share one row when "compact folders" is on.

use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::file_row::FileRow;
use crate::shifts::Shifts;

fn is_open_directory(row: &FileRow) -> bool {
    row.kind == FileKind::Directory(Expansion::Expanded)
}

fn is_directory(row: &FileRow) -> bool {
    matches!(row.kind, FileKind::Directory(_))
}

fn single_directory_child(
    head: &FileRow,
    below: &[FileRow],
) -> bool {
    let child_depth = head.depth.saturating_add(1);
    let is_child = below
        .first()
        .is_some_and(|row| row.depth == child_depth && is_directory(row));
    let sibling = below.iter().skip(1).find(|row| row.depth <= child_depth);
    is_child && sibling.is_none_or(|row| row.depth <= head.depth)
}

fn extends(rows: &[FileRow]) -> bool {
    rows.split_first()
        .is_some_and(|(head, below)| is_open_directory(head) && single_directory_child(head, below))
}

fn chain_len(rows: &[FileRow]) -> usize {
    let links = (0..rows.len())
        .take_while(|from| rows.get(*from..).is_some_and(extends))
        .count();
    links + 1
}

fn joined(chain: &[FileRow]) -> String {
    let names: Vec<&str> = chain.iter().map(|row| row.name.as_str()).collect();
    names.join("/")
}

fn next_row<'a>(
    rest: &'a [FileRow],
    shifts: &mut Shifts,
) -> Option<(FileRow, &'a [FileRow])> {
    let (chain, after) = rest.split_at_checked(chain_len(rest))?;
    let (first, last) = (chain.first()?, chain.last()?);
    let shift = shifts.at(first.depth);
    shifts.add((last.depth, chain.len() - 1));
    let row = FileRow {
        name: joined(chain),
        depth: first.depth.saturating_sub(shift),
        ..last.clone()
    };
    Some((row, after))
}

pub(crate) fn compacted(rows: &[FileRow]) -> Vec<FileRow> {
    let mut shifts = Shifts::default();
    let mut rest = rows;
    std::iter::from_fn(|| {
        let (row, after) = next_row(rest, &mut shifts)?;
        rest = after;
        Some(row)
    })
    .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::git_status::GitStatus;

    fn row(
        path: &str,
        depth: u8,
        kind: FileKind,
    ) -> FileRow {
        FileRow {
            path: path.into(),
            name: path.rsplit('/').next().unwrap_or(path).into(),
            depth,
            kind,
            status: GitStatus::Clean,
        }
    }

    fn shown(rows: &[FileRow]) -> Vec<(String, u8)> {
        compacted(rows)
            .into_iter()
            .map(|row| (row.name, row.depth))
            .collect()
    }

    #[test]
    fn single_child_folder_chains_share_a_row_and_deeper_rows_move_up() {
        let open = FileKind::Directory(Expansion::Expanded);
        let shut = FileKind::Directory(Expansion::Collapsed);
        let chain = vec![
            row("a", 0, open),
            row("a/b", 1, open),
            row("a/b/c", 2, open),
            row("a/b/c/x.rs", 3, FileKind::File),
            row("a/b/c/y.rs", 3, FileKind::File),
            row("z", 0, open),
            row("z/p", 1, open),
            row("z/p/q.rs", 2, FileKind::File),
            row("z/r.rs", 1, FileKind::File),
            row("w", 0, shut),
        ];
        let expected = [
            ("a/b/c", 0),
            ("x.rs", 1),
            ("y.rs", 1),
            ("z", 0),
            ("p", 1),
            ("q.rs", 2),
            ("r.rs", 1),
            ("w", 0),
        ];
        let expected: Vec<(String, u8)> = expected
            .iter()
            .map(|(name, depth)| ((*name).to_owned(), *depth))
            .collect();
        assert_eq!(shown(&chain), expected);
    }

    #[test]
    fn a_chain_stops_at_a_folded_folder_and_keeps_the_deepest_path() {
        let open = FileKind::Directory(Expansion::Expanded);
        let shut = FileKind::Directory(Expansion::Collapsed);
        let rows = vec![row("a", 0, open), row("a/b", 1, shut)];
        let out = compacted(&rows);
        assert_eq!(out.len(), 1);
        assert_eq!(
            out.first()
                .map(|row| (row.path.as_str(), row.name.as_str())),
            Some(("a/b", "a/b"))
        );
    }
}
