//! The menu behind the funnel button of the Files panel's search box.

use ui::IconName;

use crate::check::Check;
use crate::command::Command;
use crate::file_tree_flag::FileTreeFlag;
use crate::file_tree_view::FileTreeView;
use crate::intent::Intent;
use crate::menu_entry::MenuEntry;

fn state(
    view: &FileTreeView,
    flag: FileTreeFlag,
) -> Check {
    if view.flag(flag) {
        return Check::On;
    }
    Check::Off
}

fn entry(
    label: &str,
    icon: IconName,
    (view, flag): (&FileTreeView, FileTreeFlag),
) -> MenuEntry {
    let check = state(view, flag);
    let command = Command::SetFileTreeFlag {
        flag,
        on: check == Check::Off,
    };
    MenuEntry::toggle(label, icon, (check, Intent::Send(command)))
}

pub(crate) fn filter_menu(view: &FileTreeView) -> Vec<MenuEntry> {
    vec![
        entry("Hidden Files", IconName::Eye, (view, FileTreeFlag::Hidden)),
        entry(
            "Gitignored Files",
            IconName::GitBranch,
            (view, FileTreeFlag::Gitignored),
        ),
        MenuEntry::Separator,
        entry(
            "Git Status",
            IconName::GitBranch,
            (view, FileTreeFlag::GitStatus),
        ),
    ]
}

#[cfg(test)]
mod tests {
    use super::*;

    type Sent = (FileTreeFlag, bool, Option<Check>);

    fn sends(
        entries: &[MenuEntry],
        index: usize,
    ) -> Option<Sent> {
        let MenuEntry::Action {
            intent: Intent::Send(Command::SetFileTreeFlag { flag, on }),
            checked,
            ..
        } = entries.get(index)?
        else {
            return None;
        };
        Some((*flag, *on, *checked))
    }

    #[test]
    fn hidden_files_carry_the_eye() {
        let entries = filter_menu(&FileTreeView::default());
        let icon = match entries.first() {
            Some(MenuEntry::Action { icon, .. }) => *icon,
            _ => None,
        };
        assert_eq!(icon, Some(IconName::Eye));
    }

    #[test]
    fn each_entry_shows_its_state_and_asks_for_the_opposite() {
        let mut view = FileTreeView::default();
        view.shown.remove(&FileTreeFlag::Gitignored);
        let entries = filter_menu(&view);
        assert_eq!(entries.len(), 4);
        assert_eq!(
            sends(&entries, 0),
            Some((FileTreeFlag::Hidden, false, Some(Check::On)))
        );
        assert_eq!(
            sends(&entries, 1),
            Some((FileTreeFlag::Gitignored, true, Some(Check::Off)))
        );
        assert_eq!(entries[2], MenuEntry::Separator);
        assert_eq!(
            sends(&entries, 3),
            Some((FileTreeFlag::GitStatus, false, Some(Check::On)))
        );
    }
}
