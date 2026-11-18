//! The icon popover of the active space in the switcher.

use std::rc::Rc;

use gpui::{App, AppContext as _, Context, Entity};
use sidebar::{Command, SpaceIcon};

use crate::icon_editor::IconEditor;
use crate::sidebar::Sidebar;

impl Sidebar {
    pub(crate) fn space_icon_editor(cx: &mut Context<'_, Self>) -> Entity<IconEditor> {
        let sidebar = cx.weak_entity();
        let pick = Rc::new(move |icon: SpaceIcon, cx: &mut App| {
            sidebar
                .update(cx, |this, cx| this.set_space_icon(icon, cx))
                .unwrap_or_else(|error| tracing::debug!(%error, "sidebar closed before the icon"));
        });
        cx.new(|cx| IconEditor::new(pick, cx))
    }

    fn set_space_icon(
        &mut self,
        icon: SpaceIcon,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(project) = self.data.active_project() else {
            return;
        };
        self.send(Command::SetSpaceIcon { project, icon }, cx);
    }

    pub(crate) fn offers_space_icon(&self) -> bool {
        let Some(project) = self.data.active_project() else {
            return false;
        };
        let icon = SpaceIcon::Default;
        self.data.serves(&Command::SetSpaceIcon { project, icon })
    }

    pub(crate) fn sync_space_icon(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(project) = self.data.active_project() else {
            return;
        };
        let icon = self.data.space_icon(&project);
        self.icon_editor
            .update(cx, |editor, _| editor.current = icon);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn picking_an_icon_sets_it_on_the_active_space(cx: &mut TestAppContext) {
        let (recorder, sidebar, cx) = open(cx, DemoData::reference());
        let icon = SpaceIcon::Emoji("🚀".into());
        sidebar.update(cx, |this, cx| this.set_space_icon(icon, cx));
        let want = Command::SetSpaceIcon {
            project: ProjectId::from("inv"),
            icon: SpaceIcon::Emoji("🚀".into()),
        };
        assert_eq!(recorder.sent(), vec![want]);
    }
}
