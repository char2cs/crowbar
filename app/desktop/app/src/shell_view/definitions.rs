//! Go to definition across files, as develop's editor opener and definition provider do: a
//! definition in another workspace file opens it at its line, and one only outside the
//! workspace says so.

use gpui::{Context, Window};
use protocol::PaneId;
use ui::{ToastContent, ToastKind, ToastLifetime};

use super::ShellView;
use super::terminal_files::at_line;
use crate::panes::ide::tab_looks;
use crate::panes::kinds::pane_spec::PaneSpec;

const OUTSIDE: &str = "Definition is outside this workspace";

impl ShellView {
    pub(super) fn open_definition(
        &mut self,
        found: (&PaneId, &str, (u32, u32)),
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (pane, path, at) = found;
        let layout = self.state.layout();
        let chat = layout.pane(pane).and_then(tab_looks::file_chat_of);
        let Some(chat) = chat.map(str::to_owned) else {
            return;
        };
        let spec = at_line(PaneSpec::editor(&chat, path), at.0.checked_add(1));
        let open = spec
            .as_ref()
            .ok()
            .and_then(|spec| self.state.document_pane(spec));
        let target = open.clone().unwrap_or_else(|| pane.clone());
        self.state
            .apply(vec![layout::Command::FocusPane { pane: target }]);
        let Some(open) = open else {
            return self.open_spec(spec, cx);
        };
        self.refresh(cx);
        let editors = self.source.native().editors();
        editors.reveal((&open, at), window, cx);
        editors.focus(&open, window, cx);
    }

    pub(super) fn definition_outside(
        &self,
        path: String,
        cx: &mut Context<'_, Self>,
    ) {
        let said = ToastContent::new(ToastKind::Error, path).titled(OUTSIDE.to_owned());
        self.show_keyed(OUTSIDE, (said, ToastLifetime::Auto), cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::panes::kinds::core_panes::EDITOR;
    use crate::panes::kinds::pane_spec::PaneSpec;
    use crate::shell_view::test_support::panes_of;
    use crate::shell_view::tests::open_shell;

    #[gpui::test]
    fn a_definition_in_another_file_opens_it_at_the_definition_s_line(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        let states = shell.update_in(&mut opened.window, |shell, window, cx| {
            shell.open_spec(PaneSpec::editor("t1", "src/a.ts"), cx);
            let file = panes_of(shell, EDITOR).remove(0);
            shell.open_definition((&file, "src/b.ts", (4, 0)), window, cx);
            let layout = shell.state.layout();
            panes_of(shell, EDITOR)
                .iter()
                .filter_map(|pane| layout.pane(pane))
                .map(|instance| instance.state.as_value().get("line").cloned())
                .collect::<Vec<_>>()
        });
        assert_eq!(states.last().cloned().flatten(), Some(serde_json::json!(5)));
    }

    #[gpui::test]
    fn a_definition_in_a_file_already_open_goes_to_its_tab(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        let (focused, panes, b) = shell.update_in(&mut opened.window, |shell, window, cx| {
            shell.open_spec(PaneSpec::editor("t1", "src/b.ts"), cx);
            shell.open_spec(PaneSpec::editor("t1", "src/a.ts"), cx);
            let files = panes_of(shell, EDITOR);
            let (b, a) = (files[0].clone(), files[1].clone());
            shell.open_definition((&a, "src/b.ts", (4, 0)), window, cx);
            let focused = shell
                .state
                .showing()
                .and_then(layout::Tab::focused)
                .cloned();
            (focused, panes_of(shell, EDITOR).len(), b)
        });
        assert_eq!((focused, panes), (Some(b), 2));
    }

    #[gpui::test]
    fn a_definition_only_outside_the_workspace_says_so(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        let before = shell.update(&mut opened.window, |shell, cx| shell.notices.read(cx).len());
        shell.update(&mut opened.window, |shell, cx| {
            shell.definition_outside("/usr/lib/node/lib.d.ts".to_owned(), cx);
        });
        opened.window.run_until_parked();
        let after = shell.update(&mut opened.window, |shell, cx| shell.notices.read(cx).len());
        assert_eq!(after - before, 1);
    }
}
