use chord::{Keystroke, Modifier, Modifiers, PhysicalKey};
use gpui::{Context, KeyDownEvent, Window};
use keymap::ShellCommand;
use layout::Command;
use sidebar::{Mode, Sink as _, SpaceList as _};

use crate::shell::shell_view::ShellView;

mod dialogs;
mod new_chats;
use crate::panes::tracking::group_focus;
use crate::shell::shell_view::commands::direction::Direction;
use crate::shell::shell_view::commands::focus_neighbour::focus_neighbour;
use crate::shell::shell_view::commands::project_slot::project_at;

const UNIT_EXTENT: f32 = 1000.0;
const PRIMARY_IS_COMMAND: bool = cfg!(target_os = "macos");
const RELATIVE_BOUNDS: layout::Rect = layout::Rect {
    x: 0.0,
    y: 0.0,
    width: UNIT_EXTENT,
    height: UNIT_EXTENT,
};

impl ShellView {
    pub(crate) fn on_key_down(
        &mut self,
        event: &KeyDownEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let stroke = heard(&event.keystroke, cx);
        let Some(command) = self.keymap.resolve(&stroke) else {
            return self.on_unbound_key(&stroke, window, cx);
        };
        cx.stop_propagation();
        self.run_shell_command(command, window, cx);
    }

    pub(crate) fn run_shell_command(
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
            ShellCommand::SaveAll => self.request_saves(&self.state.dirty().all()),
            ShellCommand::ZoomChatIn | ShellCommand::ZoomChatOut | ShellCommand::ZoomChatReset => {
                self.state.zoom_chat(command);
            },
        }
    }

    pub(crate) fn switch_project_on(
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

    pub(crate) fn select_project(
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

    fn focus_towards(
        &mut self,
        direction: Direction,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = self.state.showing() else {
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
        let target = self.state.showing().and_then(group_focus::close_target);
        if let Some(pane) = target {
            self.apply_guarded(vec![Command::ClosePane { pane }], cx);
        }
    }
}

pub(crate) fn heard(
    stroke: &gpui::Keystroke,
    cx: &gpui::App,
) -> Keystroke {
    keystroke_of(stroke, physical_of(stroke, cx))
}

#[cfg(target_os = "linux")]
fn physical_of(
    stroke: &gpui::Keystroke,
    cx: &gpui::App,
) -> Option<PhysicalKey> {
    crate::adapters::key_places::of(stroke, modifiers_of(stroke.modifiers), cx)
}

// PLATFORM: macos names a shifted key by its own place already; no lookup is needed there.
#[cfg(not(target_os = "linux"))]
fn physical_of(
    _stroke: &gpui::Keystroke,
    _cx: &gpui::App,
) -> Option<PhysicalKey> {
    None
}

pub(crate) fn keystroke_of(
    stroke: &gpui::Keystroke,
    physical: Option<PhysicalKey>,
) -> Keystroke {
    Keystroke {
        modifiers: modifiers_of(stroke.modifiers),
        key: stroke.key.clone(),
        physical,
    }
}

fn modifiers_of(modifiers: gpui::Modifiers) -> Modifiers {
    let primary =
        (PRIMARY_IS_COMMAND && modifiers.platform) || (!PRIMARY_IS_COMMAND && modifiers.control);
    let held = [
        (primary, Modifier::Primary),
        (modifiers.shift, Modifier::Shift),
        (modifiers.alt, Modifier::Alt),
    ];
    held.into_iter()
        .filter(|(down, _)| *down)
        .map(|(_, modifier)| modifier)
        .collect()
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
    fn a_digit_key_typing_a_symbol_under_shift_still_selects_its_project() {
        let typed = gpui::Keystroke::parse_with_os("secondary-@", native_os()).unwrap();
        let held = gpui::Keystroke::parse_with_os("secondary-shift-2", native_os()).unwrap();
        let physical = PhysicalKey {
            code: held.key,
            modifiers: modifiers_of(held.modifiers),
        };
        assert_eq!(
            keymap::project_slot_of_stroke(&keystroke_of(&typed, Some(physical))),
            Some(2)
        );
    }

    #[test]
    fn a_typed_key_becomes_a_chord() {
        let stroke = gpui::Keystroke::parse_with_os("secondary-shift-t", native_os()).unwrap();
        assert_eq!(
            keystroke_of(&stroke, None).chord().as_deref(),
            Some("mod+shift+t")
        );
    }
}
