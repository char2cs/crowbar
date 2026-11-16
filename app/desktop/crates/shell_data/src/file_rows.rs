//! The visible rows of the Files panel.

use sidebar::{Expansion, FileKind, FileRow, GitStatus};

use crate::file_entry::FileEntry;
use crate::panel_state::PanelState;

pub(crate) fn of(panel: &PanelState) -> Vec<FileRow> {
    level(panel, "", 0)
}

fn level(
    panel: &PanelState,
    directory: &str,
    depth: u8,
) -> Vec<FileRow> {
    let entries = panel.listings.get(directory).map_or_default(Vec::as_slice);
    entries
        .iter()
        .flat_map(|entry| entry_rows(panel, entry, depth))
        .collect()
}

fn entry_rows(
    panel: &PanelState,
    entry: &FileEntry,
    depth: u8,
) -> Vec<FileRow> {
    let expansion = expansion_of(panel, entry);
    let mut rows = vec![row(entry, (expansion, depth), panel)];
    if entry.is_directory() && expansion == Expansion::Expanded {
        rows.extend(level(panel, &entry.path, depth.saturating_add(1)));
    }
    rows
}

fn expansion_of(
    panel: &PanelState,
    entry: &FileEntry,
) -> Expansion {
    if panel.expanded.contains(&entry.path) {
        return Expansion::Expanded;
    }
    Expansion::Collapsed
}

fn row(
    entry: &FileEntry,
    (expansion, depth): (Expansion, u8),
    panel: &PanelState,
) -> FileRow {
    FileRow {
        path: entry.path.clone(),
        name: entry.name.clone(),
        depth,
        kind: kind_of(entry, expansion),
        status: status_of(entry, panel),
    }
}

fn kind_of(
    entry: &FileEntry,
    expansion: Expansion,
) -> FileKind {
    if entry.is_directory() {
        return FileKind::Directory(expansion);
    }
    FileKind::File
}

fn status_of(
    entry: &FileEntry,
    panel: &PanelState,
) -> GitStatus {
    let (path, directory) = (entry.path.as_str(), entry.is_directory());
    let statuses = &panel.statuses;
    if let Some(status) = statuses.get(path) {
        return *status;
    }
    if panel.ignores.ignores(path, entry.entry_kind()) {
        return GitStatus::Ignored;
    }
    let inside = |changed: &str| directory && changed.starts_with(&format!("{path}/"));
    if statuses.keys().any(|changed| inside(changed)) {
        return GitStatus::Modified;
    }
    statuses
        .iter()
        .find(|(changed, _)| path.starts_with(&format!("{changed}/")))
        .map_or(GitStatus::Clean, |(_, status)| *status)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn entry(
        name: &str,
        path: &str,
        kind: &str,
    ) -> FileEntry {
        FileEntry {
            name: name.into(),
            path: path.into(),
            kind: kind.into(),
        }
    }

    fn panel() -> PanelState {
        let mut panel = PanelState::default();
        panel.listings.insert(
            String::new(),
            vec![
                entry("src", "src", "directory"),
                entry("a.txt", "a.txt", "file"),
            ],
        );
        panel
            .listings
            .insert("src".into(), vec![entry("m.rs", "src/m.rs", "file")]);
        panel.statuses.insert("a.txt".into(), GitStatus::Untracked);
        panel.statuses.insert("src".into(), GitStatus::Untracked);
        panel
    }

    #[test]
    fn a_folded_directory_hides_its_children_and_an_unfolded_one_shows_them_deeper() {
        let mut panel = panel();
        assert_eq!(of(&panel).len(), 2);
        panel.toggle("src");
        let rows = of(&panel);
        assert_eq!(
            rows.iter()
                .map(|row| (row.path.as_str(), row.depth))
                .collect::<Vec<_>>(),
            [("src", 0), ("src/m.rs", 1), ("a.txt", 0)]
        );
        assert_eq!(rows[0].kind, FileKind::Directory(Expansion::Expanded));
    }

    #[test]
    fn status_is_inherited_from_an_untracked_directory_and_bubbles_to_a_parent() {
        let mut panel = panel();
        panel.toggle("src");
        let rows = of(&panel);
        assert_eq!(rows[1].status, GitStatus::Untracked);
        panel.statuses.clear();
        panel.statuses.insert("src/m.rs".into(), GitStatus::Added);
        assert_eq!(of(&panel)[0].status, GitStatus::Modified);
    }

    #[test]
    fn a_path_the_gitignore_hides_reads_as_ignored_unless_git_reports_it() {
        let mut panel = panel();
        panel.toggle("src");
        panel.statuses.clear();
        panel.statuses.insert("a.txt".into(), GitStatus::Untracked);
        panel.ignores.learn("", "src/\na.txt");
        let rows = of(&panel);
        let statuses: Vec<_> = rows.iter().map(|row| row.status).collect();
        assert_eq!(
            statuses,
            [GitStatus::Ignored, GitStatus::Ignored, GitStatus::Untracked]
        );
    }
}
