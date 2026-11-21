use chord::{Keystroke, Modifier};
use gpui::{Context, KeyDownEvent, Window};
use keymap::ShellCommand;
use layout::Command;
use sidebar::{ActiveSpace as _, Mode, PaneFocus as _, Sink as _, SpaceList as _};

use super::ShellView;

mod dialogs;
use crate::direction::Direction;
use crate::focus_neighbour::focus_neighbour;
use crate::project_slot::project_at;

const UNIT_EXTENT: f32 = 1000.0;
const PRIMARY_IS_COMMAND: bool = cfg!(target_os = "macos");
const RELATIVE_BOUNDS: layout::Rect = layout::Rect {
    x: 0.0,
    y: 0.0,
    width: UNIT_EXTENT,
    height: UNIT_EXTENT,
};

impl ShellView {
    pub(super) fn on_key_down(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let stroke = keystroke_of(&event.keystroke);
        let Some(command) = self.keymap.resolve(&stroke) else {
            return self.on_unbound_key(&stroke, window, cx);
        };
        cx.stop_propagation();
        self.run_shell_command(command, window, cx);
    }

    pub(super) fn run_shell_command(
        &mut self,
        command: ShellCommand,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        match command {
            ShellCommand::ToggleEditor => self.toggle_editor(cx),
            ShellCommand::ToggleSidebar => {
                self.sidebar
                    .update(cx, |sidebar, cx| sidebar.toggle(window, cx));
            },
            ShellCommand::SidebarChats => self.set_sidebar_mode(Mode::Chats, cx),
            ShellCommand::SidebarFiles => self.set_sidebar_mode(Mode::Files, cx),
            ShellCommand::SidebarGit => self.set_sidebar_mode(Mode::Changes, cx),
            ShellCommand::CloseTab => self.close_focused(cx),
            ShellCommand::ReopenClosedTab => self.reopen_closed_tab(cx),
            ShellCommand::NewTerminal => self.new_terminal(window, cx),
            ShellCommand::NewBrowser => self.new_browser(cx),
            ShellCommand::NewFile => self.new_file(cx),
            ShellCommand::OpenFile => self.open_file_from_disk(window, cx),
            ShellCommand::OpenReview => self.open_review(cx),
            ShellCommand::ToggleConsole => self.toggle_console(cx),
            ShellCommand::OpenLog => self.open_log(),
            ShellCommand::ExportDiagnostics => self.export_diagnostics(),
            ShellCommand::NewTab => self.new_tab(cx),
            ShellCommand::NewChat => self.new_chat(cx),
            ShellCommand::NewChatTerminal => self.new_terminal_chat(),
            ShellCommand::FocusPaneLeft => self.focus_towards(Direction::Left, cx),
            ShellCommand::FocusPaneRight => self.focus_towards(Direction::Right, cx),
            ShellCommand::FocusPaneUp => self.focus_towards(Direction::Up, cx),
            ShellCommand::FocusPaneDown => self.focus_towards(Direction::Down, cx),
        }
    }

    pub(super) fn switch_project_on(
        &self,
        stroke: &Keystroke,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(slot) = keymap::project_slot_of_stroke(stroke) else {
            return;
        };
        cx.stop_propagation();
        self.select_project(slot);
    }

    pub(super) fn select_project(
        &self,
        slot: u8,
    ) {
        let view = self.data.view();
        let projects = view.projects();
        let Some(project) = project_at(&projects, slot) else {
            return;
        };
        if let Err(error) = self
            .data
            .commands()
            .dispatch(sidebar::Command::SelectProject(project.id.clone()))
        {
            tracing::warn!(%error, "the project switch was refused");
        }
    }

    fn new_chat(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if crate::group_focus::editor_focused(self.state.layout()) {
            return self.new_tab(cx);
        }
        self.start_chat(sidebar::Command::NewThread);
    }

    fn new_terminal_chat(&self) {
        let terminal_ok = self.data.view().can_start_terminal_thread();
        if terminal_ok {
            return self.start_chat(sidebar::Command::NewTerminalThread);
        }
        self.start_chat(sidebar::Command::NewThread);
    }

    fn start_chat(
        &self,
        create: fn(sidebar::ProjectId) -> sidebar::Command,
    ) {
        let Some(project) = self.data.view().active_project() else {
            tracing::info!("a new chat needs a project to be selected first");
            return;
        };
        if let Err(error) = self.data.commands().dispatch(create(project)) {
            tracing::warn!(%error, "the new chat was refused");
        }
    }

    fn focus_towards(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = self.state.layout().active_tab() else {
            return;
        };
        let Some(from) = tab.focused() else {
            return;
        };
        let slots = tab.layout(RELATIVE_BOUNDS);
        if let Some(pane) = focus_neighbour(&slots, from, direction) {
            self.state.apply(vec![Command::FocusPane { pane }]);
            self.refresh(cx);
        }
    }

    fn set_sidebar_mode(
        &mut self,
        mode: Mode,
        cx: &mut Context<'_, Self>,
    ) {
        self.sidebar
            .update(cx, |sidebar, cx| sidebar.set_mode(mode, cx));
    }

    fn close_focused(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let target = self
            .state
            .layout()
            .active_tab()
            .and_then(crate::group_focus::close_target);
        if let Some(pane) = target {
            self.apply_guarded(vec![Command::ClosePane { pane }], cx);
        }
    }
}

fn keystroke_of(stroke: &gpui::Keystroke) -> Keystroke {
    let modifiers = stroke.modifiers;
    let primary =
        (PRIMARY_IS_COMMAND && modifiers.platform) || (!PRIMARY_IS_COMMAND && modifiers.control);
    let held = [
        (primary, Modifier::Primary),
        (modifiers.shift, Modifier::Shift),
        (modifiers.alt, Modifier::Alt),
    ];
    Keystroke {
        modifiers: held
            .into_iter()
            .filter(|(down, _)| *down)
            .map(|(_, modifier)| modifier)
            .collect(),
        key: stroke.key.clone(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn native_os() -> gpui::OperatingSystem {
        if cfg!(target_os = "macos") {
            return gpui::OperatingSystem::Mac;
        }
        gpui::OperatingSystem::Linux
    }

    #[test]
    fn a_typed_key_becomes_a_chord() {
        let stroke = gpui::Keystroke::parse_with_os("secondary-shift-t", native_os()).unwrap();
        assert_eq!(
            keystroke_of(&stroke).chord().as_deref(),
            Some("mod+shift+t")
        );
    }
}
