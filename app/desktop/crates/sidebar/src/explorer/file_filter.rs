mod hidden_pattern;

use crate::explorer::file_filter::hidden_pattern::matches;
use crate::shell::FileKind;
use crate::shell::FileRow;
use crate::shell::FileTreeFlag;
use crate::shell::FileTreeView;
use crate::shell::GitStatus;

const ALWAYS_HIDDEN: &str = ".ds_store";

fn is_directory(row: &FileRow) -> bool {
    matches!(row.kind, FileKind::Directory(_))
}

fn is_dotfile(name: &str) -> bool {
    name.starts_with('.') && name.len() > 1
}

fn hides(
    row: &FileRow,
    view: &FileTreeView,
) -> bool {
    if row.name.eq_ignore_ascii_case(ALWAYS_HIDDEN) {
        return true;
    }
    if !view.flag(FileTreeFlag::Hidden) && is_dotfile(&row.name) {
        return true;
    }
    if !view.flag(FileTreeFlag::Gitignored) && row.status == GitStatus::Ignored {
        return true;
    }
    let by_file = view
        .hidden_files
        .iter()
        .any(|pattern| matches(pattern, &row.path, row.kind));
    by_file
        || (is_directory(row)
            && view
                .hidden_directories
                .iter()
                .any(|pattern| matches(pattern, &row.path, row.kind)))
}

fn decorated(
    mut row: FileRow,
    view: &FileTreeView,
) -> FileRow {
    if !view.flag(FileTreeFlag::GitStatus) && row.status != GitStatus::Ignored {
        row.status = GitStatus::Clean;
    }
    row
}

fn keep(
    row: FileRow,
    view: &FileTreeView,
    skipping: &mut Option<u8>,
) -> Option<FileRow> {
    if skipping.is_some_and(|depth| row.depth > depth) {
        return None;
    }
    *skipping = None;
    if hides(&row, view) {
        *skipping = is_directory(&row).then_some(row.depth);
        return None;
    }
    Some(decorated(row, view))
}

impl FileTreeView {
    pub fn visible(
        &self,
        rows: Vec<FileRow>,
    ) -> Vec<FileRow> {
        let mut skipping = None;
        rows.into_iter()
            .filter_map(|row| keep(row, self, &mut skipping))
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::Expansion;

    fn row(
        path: &str,
        depth: u8,
        kind: FileKind,
        status: GitStatus,
    ) -> FileRow {
        FileRow {
            path: path.into(),
            name: path.rsplit('/').next().unwrap_or(path).into(),
            depth,
            kind,
            status,
        }
    }

    fn tree() -> Vec<FileRow> {
        let open = FileKind::Directory(Expansion::Expanded);
        vec![
            row(".github", 0, open, GitStatus::Clean),
            row(".github/ci.yml", 1, FileKind::File, GitStatus::Clean),
            row("src", 0, open, GitStatus::Clean),
            row("src/main.rs", 1, FileKind::File, GitStatus::Modified),
            row("src/run.log", 1, FileKind::File, GitStatus::Clean),
            row("src/.DS_Store", 1, FileKind::File, GitStatus::Clean),
            row("target", 0, open, GitStatus::Ignored),
            row("target/out.bin", 1, FileKind::File, GitStatus::Ignored),
            row("vendor", 0, open, GitStatus::Clean),
            row("vendor/lib.rs", 1, FileKind::File, GitStatus::Clean),
            row("README.md", 0, FileKind::File, GitStatus::Added),
        ]
    }

    fn paths(rows: Vec<FileRow>) -> Vec<String> {
        rows.into_iter().map(|row| row.path).collect()
    }

    fn turn_off(
        view: &mut FileTreeView,
        flag: FileTreeFlag,
    ) {
        view.shown.remove(&flag);
    }

    fn view(change: impl FnOnce(&mut FileTreeView)) -> FileTreeView {
        let mut view = FileTreeView::default();
        change(&mut view);
        view
    }

    type Setup = fn(&mut FileTreeView);
    type Case = (&'static str, Setup, &'static [&'static str]);

    fn hide_files(view: &mut FileTreeView) {
        view.hidden_files = vec!["*.log".into()];
    }

    fn hide_directories(view: &mut FileTreeView) {
        view.hidden_directories = vec!["vendor".into(), "README.md".into()];
    }

    #[test]
    fn the_settings_decide_which_rows_stay_and_children_follow_their_folder() {
        let cases: [Case; 5] = [
            ("defaults hide only OS junk", |_| {}, &["src/.DS_Store"]),
            (
                "hidden off",
                |v| turn_off(v, FileTreeFlag::Hidden),
                &[".github", ".github/ci.yml", "src/.DS_Store"],
            ),
            (
                "gitignored off",
                |v| turn_off(v, FileTreeFlag::Gitignored),
                &["src/.DS_Store", "target", "target/out.bin"],
            ),
            (
                "file patterns",
                hide_files,
                &["src/.DS_Store", "src/run.log"],
            ),
            (
                "directory patterns",
                hide_directories,
                &["src/.DS_Store", "vendor", "vendor/lib.rs"],
            ),
        ];
        for (name, setup, hidden) in cases {
            let expected: Vec<String> = paths(tree())
                .into_iter()
                .filter(|path| !hidden.contains(&path.as_str()))
                .collect();
            assert_eq!(paths(view(setup).visible(tree())), expected, "{name}");
        }
    }

    #[test]
    fn git_status_off_clears_the_letters_but_keeps_ignored_files_dimmed() {
        let rows = view(|view| turn_off(view, FileTreeFlag::GitStatus)).visible(tree());
        let statuses: Vec<GitStatus> = rows.iter().map(|row| row.status).collect();
        assert!(
            statuses
                .iter()
                .all(|status| matches!(status, GitStatus::Clean | GitStatus::Ignored))
        );
        assert!(statuses.contains(&GitStatus::Ignored));
    }
}
