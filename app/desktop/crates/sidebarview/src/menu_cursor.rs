//! Which entry of an open menu is highlighted and which submenu is showing.

use sidebar::Intent;

use crate::menu_entry::MenuEntry;
use crate::menu_step::MenuStep;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) struct MenuCursor {
    highlighted: Option<usize>,
}

impl MenuCursor {
    pub(crate) fn highlighted(self) -> Option<usize> {
        self.highlighted
    }

    pub(crate) fn highlight(
        &mut self,
        index: Option<usize>,
    ) {
        self.highlighted = index;
    }

    pub(crate) fn open_submenu(
        self,
        entries: &[MenuEntry],
    ) -> Option<usize> {
        let index = self.highlighted?;
        matches!(entries.get(index), Some(MenuEntry::Submenu { .. })).then_some(index)
    }

    pub(crate) fn step(
        &mut self,
        step: MenuStep,
        entries: &[MenuEntry],
    ) {
        let selectable = |index: &usize| !matches!(entries.get(*index), Some(MenuEntry::Separator));
        let candidates: Vec<usize> = (0..entries.len()).filter(selectable).collect();
        let next = match (step, self.highlighted) {
            (MenuStep::Next, None) => candidates.first().copied(),
            (MenuStep::Previous, None) => candidates.last().copied(),
            (MenuStep::Next, Some(from)) => candidates.iter().copied().find(|index| *index > from),
            (MenuStep::Previous, Some(from)) => {
                candidates.iter().rev().copied().find(|index| *index < from)
            },
        };
        self.highlighted = next.or(self.highlighted);
    }

    pub(crate) fn chosen(
        self,
        entries: &[MenuEntry],
    ) -> Option<Intent> {
        match entries.get(self.highlighted?)? {
            MenuEntry::Action { intent, .. } => Some(intent.clone()),
            MenuEntry::Separator | MenuEntry::Submenu { .. } => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use ui::IconName;

    use super::*;
    use sidebar::RowId;

    fn entries() -> Vec<MenuEntry> {
        let rename = Intent::StartRename(RowId::from("r"));
        vec![
            MenuEntry::action("Rename", IconName::PencilSimpleLine, rename),
            MenuEntry::Separator,
            MenuEntry::Submenu {
                label: "Move to…".into(),
                icon: None,
                entries: Vec::new(),
            },
            MenuEntry::destructive("Delete", IconName::Trash, Intent::Keep(RowId::from("r"))),
        ]
    }

    type StepCase = (&'static str, &'static [MenuStep], Option<usize>);

    const STEP_CASES: &[StepCase] = &[
        ("first selectable", &[MenuStep::Next], Some(0)),
        (
            "separator skipped",
            &[MenuStep::Next, MenuStep::Next],
            Some(2),
        ),
        (
            "stays on the last entry",
            &[
                MenuStep::Next,
                MenuStep::Next,
                MenuStep::Next,
                MenuStep::Next,
            ],
            Some(3),
        ),
        (
            "previous from nothing is the last",
            &[MenuStep::Previous],
            Some(3),
        ),
        (
            "previous again skips the separator",
            &[MenuStep::Previous, MenuStep::Previous],
            Some(2),
        ),
    ];

    fn walk(
        steps: &[MenuStep],
        list: &[MenuEntry],
    ) -> MenuCursor {
        let mut cursor = MenuCursor::default();
        for step in steps {
            cursor.step(*step, list);
        }
        cursor
    }

    #[test]
    fn stepping_skips_separators_and_stops_at_the_ends() {
        let list = entries();
        for (name, steps, expected) in STEP_CASES {
            let cursor = walk(steps, &list);
            assert_eq!(cursor.highlighted(), *expected, "{name}");
        }
    }

    type Pick = (&'static str, Option<usize>, Option<usize>, Option<Intent>);

    fn picks() -> Vec<Pick> {
        let rename = Intent::StartRename(RowId::from("r"));
        vec![
            (
                "an action opens nothing and is chosen",
                Some(0),
                None,
                Some(rename),
            ),
            ("a submenu opens and is not chosen", Some(2), Some(2), None),
            ("nothing highlighted", None, None, None),
        ]
    }

    #[test]
    fn only_a_highlighted_submenu_opens_and_only_an_action_is_chosen() {
        let list = entries();
        for (name, highlighted, opens, chosen) in picks() {
            let mut cursor = MenuCursor::default();
            cursor.highlight(highlighted);
            assert_eq!(cursor.open_submenu(&list), opens, "{name}: opens");
            assert_eq!(cursor.chosen(&list), chosen, "{name}: chosen");
        }
    }
}
