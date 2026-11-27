//! The menu behind the funnel button of the Files panel's search box.

use sidebar::{Command, FileTreeFlag, FileTreeView, Intent};

use crate::overlays::popup::check::Check;
use crate::overlays::popup::menu_entry::MenuEntry;

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
    (view, flag): (&FileTreeView, FileTreeFlag),
) -> MenuEntry {
    let check = state(view, flag);
    let command = Command::SetFileTreeFlag {
        flag,
        on: check == Check::Off,
    };
    MenuEntry::toggle(label, (check, Intent::Send(command)))
}

pub(crate) fn filter_menu(view: &FileTreeView) -> Vec<MenuEntry> {
    vec![
        entry("Hidden Files", (view, FileTreeFlag::Hidden)),
        entry("Gitignored Files", (view, FileTreeFlag::Gitignored)),
        MenuEntry::Separator,
        entry("Git Status", (view, FileTreeFlag::GitStatus)),
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
