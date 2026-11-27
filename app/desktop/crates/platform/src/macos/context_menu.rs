//! A native `NSMenu`, popped over a window's view.

use objc2::rc::Retained;
use objc2::{MainThreadMarker, MainThreadOnly as _};
use objc2_app_kit::{NSControlStateValueOn, NSMenu, NSMenuItem};
use objc2_foundation::{NSPoint, NSString};

use crate::macos::appkit_parent::AppkitParent;
use crate::macos::menu_pick::MenuPick;
use crate::macos::menu_row::MenuRow;

/// A menu ready to pop up, which remembers the row picked from it.
#[derive(Debug)]
pub struct ContextMenu {
    menu: Retained<NSMenu>,
    target: Retained<MenuPick>,
    paths: Vec<Vec<usize>>,
}

impl ContextMenu {
    /// The menu of `rows`, top to bottom.
    #[must_use]
    pub fn new(
        rows: &[MenuRow],
        main_thread: MainThreadMarker,
    ) -> Self {
        let mut built = Self {
            menu: empty(main_thread),
            target: MenuPick::new(main_thread),
            paths: Vec::new(),
        };
        let menu = built.menu.clone();
        built.fill(&menu, rows, &[]);
        built
    }

    /// The `AppKit` menu.
    #[must_use]
    pub fn menu(&self) -> &NSMenu {
        &self.menu
    }

    /// The picked row: its index in the menu, then in each submenu it lies in.
    #[must_use]
    pub fn picked(&self) -> Option<Vec<usize>> {
        let tag = usize::try_from(self.target.tag()?).ok()?;
        self.paths.get(tag).cloned()
    }

    /// Opens at `(x, y)` from the top-left of `view` and tracks until picked or dismissed.
    pub fn show(
        &self,
        view: &AppkitParent,
        (x, y): (f64, f64),
    ) -> Option<Vec<usize>> {
        let view = view.view();
        let unflipped = (!view.isFlipped()).then(|| view.frame().size.height);
        let at = NSPoint::new(x, from_top(y, unflipped));
        self.menu
            .popUpMenuPositioningItem_atLocation_inView(None, at, Some(view));
        self.picked()
    }

    fn fill(
        &mut self,
        menu: &NSMenu,
        rows: &[MenuRow],
        under: &[usize],
    ) {
        for (index, row) in rows.iter().enumerate() {
            let path = [under, &[index]].concat();
            menu.addItem(&self.item(row, path));
        }
    }

    fn item(
        &mut self,
        row: &MenuRow,
        path: Vec<usize>,
    ) -> Retained<NSMenuItem> {
        let main_thread = self.menu.mtm();
        match row {
            MenuRow::Separator => NSMenuItem::separatorItem(main_thread),
            MenuRow::Item {
                label,
                enabled,
                checked,
            } => {
                let item = titled(label, main_thread);
                item.setEnabled(*enabled);
                item.setState(isize::from(*checked) * NSControlStateValueOn);
                self.aim(&item, path);
                item
            },
            MenuRow::Submenu {
                label,
                enabled,
                rows,
            } => {
                let item = titled(label, main_thread);
                item.setEnabled(*enabled);
                let nested = empty(main_thread);
                self.fill(&nested, rows, &path);
                item.setSubmenu(Some(&nested));
                item
            },
        }
    }

    fn aim(
        &mut self,
        item: &NSMenuItem,
        path: Vec<usize>,
    ) {
        item.setTag(self.paths.len().cast_signed());
        self.target.aim(item);
        self.paths.push(path);
    }
}

fn empty(main_thread: MainThreadMarker) -> Retained<NSMenu> {
    let menu = NSMenu::new(main_thread);
    menu.setAutoenablesItems(false);
    menu
}

fn titled(
    label: &str,
    main_thread: MainThreadMarker,
) -> Retained<NSMenuItem> {
    let item = NSMenuItem::new(main_thread);
    item.setTitle(&NSString::from_str(label));
    item
}

fn from_top(
    y: f64,
    unflipped_height: Option<f64>,
) -> f64 {
    unflipped_height.map_or(y, |height| height - y)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_point_is_counted_from_the_top_of_either_kind_of_view() {
        let cases = [("flipped", None, 10.0), ("not flipped", Some(100.0), 90.0)];
        for (name, unflipped, want) in cases {
            let y = from_top(10.0, unflipped);
            assert!((y - want).abs() < f64::EPSILON, "{name}: {y}");
        }
    }
}
