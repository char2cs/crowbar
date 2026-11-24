//! Go to definition across files, as develop's editor opener and definition provider do: a
//! definition in another workspace file opens it at its line, and one only outside the
//! workspace says so.

use gpui::Context;
use protocol::PaneId;
use ui::{ToastContent, ToastKind, ToastLifetime};

use super::ShellView;
use super::terminal_files::at_line;
use crate::pane_spec::PaneSpec;

const OUTSIDE: &str = "Definition is outside this workspace";

impl ShellView {
    pub(super) fn open_definition(
        &mut self,
        pane: &PaneId,
        (path, line): (&str, u32),
        cx: &mut Context<'_, Self>,
    ) {
        let layout = self.state.layout();
        let chat = layout.pane(pane).and_then(crate::tab_looks::file_chat_of);
        let Some(chat) = chat.map(str::to_owned) else {
            return;
        };
        let focus = layout::Command::FocusPane { pane: pane.clone() };
        self.state.apply(vec![focus]);
        let spec = at_line(PaneSpec::editor(&chat, path), line.checked_add(1));
        self.open_spec(spec, cx);
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

    use crate::core_panes::EDITOR;
    use crate::pane_spec::PaneSpec;
    use crate::shell_view::test_support::panes_of;
    use crate::shell_view::tests::open_shell;

    #[gpui::test]
    fn a_definition_in_another_file_opens_it_at_the_definition_s_line(cx: &mut TestAppContext) {
        let mut opened = open_shell(cx);
        let shell = opened.shell.clone();
        let states = shell.update_in(&mut opened.window, |shell, _, cx| {
            shell.open_spec(PaneSpec::editor("t1", "src/a.ts"), cx);
            let file = panes_of(shell, EDITOR).remove(0);
            shell.open_definition(&file, ("src/b.ts", 4), cx);
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
