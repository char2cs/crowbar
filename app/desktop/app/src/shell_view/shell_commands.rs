use std::path::Path;

use branch_import_view::{BranchImportView, ImportEvent};
use command_palette::{PaletteEntry, PaletteEvent, PalettePanel};
use gpui::{AppContext as _, Context, Entity, KeyDownEvent, Window};
use model::Command;
use pairing_view::{PairingEvent, PairingView};
use settings_ui::{Chord, Primary};
use shell_diagnostics::LogPaths;
use shell_keys::{Keystroke, ShellCommand};
use sidebar::{CommandSink as _, Mode, Sidebar, SidebarData as _};

use super::ShellView;
use crate::direction::Direction;
use crate::focus_neighbour::focus_neighbour;
use crate::project_slot::project_at;

// SPEC: only the relative placement of panes matters when looking for a neighbour.
const UNIT_EXTENT: f32 = 1000.0;
const PRIMARY_IS_COMMAND: bool = cfg!(target_os = "macos");
const RELATIVE_BOUNDS: model::Rect = model::Rect {
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
            return self.switch_project_on(&stroke, cx);
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
            ShellCommand::OpenSettings => self.show_settings(window, cx),
            ShellCommand::CommandPalette => self.show_palette(window, cx),
            ShellCommand::ToggleSidebar => self.sidebar.update(cx, Sidebar::toggle),
            ShellCommand::SidebarChats => self.set_sidebar_mode(Mode::Chats, cx),
            ShellCommand::SidebarFiles => self.set_sidebar_mode(Mode::Files, cx),
            ShellCommand::SidebarGit => self.set_sidebar_mode(Mode::Changes, cx),
            ShellCommand::CloseTab => self.close_active_tab(cx),
            ShellCommand::ReopenClosedTab => self.reopen_closed_tab(cx),
            ShellCommand::NewTerminal => self.new_terminal(cx),
            ShellCommand::NewBrowser => self.new_browser(cx),
            ShellCommand::NewFile => self.new_file(cx),
            ShellCommand::OpenFile => self.open_file_from_disk(window, cx),
            ShellCommand::OpenReview => self.open_review(cx),
            ShellCommand::ToggleConsole => self.toggle_console(cx),
            ShellCommand::OpenLog => self.open_log(),
            ShellCommand::ExportDiagnostics => self.export_diagnostics(),
            ShellCommand::NewTab => self.new_tab(cx),
            ShellCommand::NewChat => self.start_chat(sidebar::Command::NewThread),
            ShellCommand::NewChatTerminal => self.new_terminal_chat(),
            ShellCommand::FocusPaneLeft => self.focus_towards(Direction::Left, cx),
            ShellCommand::FocusPaneRight => self.focus_towards(Direction::Right, cx),
            ShellCommand::FocusPaneUp => self.focus_towards(Direction::Up, cx),
            ShellCommand::FocusPaneDown => self.focus_towards(Direction::Down, cx),
        }
    }

    fn switch_project_on(
        &self,
        stroke: &Keystroke,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(slot) = shell_keys::project_slot_of_stroke(stroke) else {
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

    // SPEC: the chat opens through `Outcome::OpenChat` once the daemon has created it.
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

    fn export_diagnostics(&self) {
        let data_dir = self.preferences_path.parent().unwrap_or(Path::new(""));
        crate::diagnostics_export::start(&self.link, LogPaths::in_data_dir(data_dir));
    }

    fn open_log(&self) {
        let data_dir = self.preferences_path.parent().unwrap_or(Path::new(""));
        let log = LogPaths::in_data_dir(data_dir).current();
        if let Err(error) = shell_platform::open_path(&log) {
            tracing::warn!(%error, "the log file could not be opened");
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

    fn close_active_tab(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = self.state.layout().active_tab().map(model::Tab::id) else {
            return;
        };
        self.apply_guarded(vec![Command::CloseTab { tab }], cx);
    }

    fn show_palette(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.palette.is_some() {
            return;
        }
        let entries = palette_entries(&self.keymap);
        let panel = cx.new(|cx| PalettePanel::new(entries, window, cx));
        self.subscriptions
            .push(cx.subscribe_in(&panel, window, Self::on_palette));
        self.palette = Some(panel);
        self.update_overlay(cx);
    }

    #[expect(
        clippy::too_many_arguments,
        reason = "the gpui subscribe_in callback shape is fixed"
    )]
    fn on_palette(
        &mut self,
        _: &Entity<PalettePanel>,
        event: &PaletteEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.palette = None;
        self.update_overlay(cx);
        let chosen = match event {
            PaletteEvent::Run(id) => ShellCommand::parse(id),
            _ => None,
        };
        if let Some(command) = chosen {
            self.run_shell_command(command, window, cx);
        }
    }
}

fn keystroke_of(stroke: &gpui::Keystroke) -> Keystroke {
    let modifiers = stroke.modifiers;
    Keystroke {
        primary: (PRIMARY_IS_COMMAND && modifiers.platform)
            || (!PRIMARY_IS_COMMAND && modifiers.control),
        shift: modifiers.shift,
        alt: modifiers.alt,
        key: stroke.key.clone(),
    }
}

fn palette_entries(keymap: &shell_keys::Keymap) -> Vec<PaletteEntry> {
    ShellCommand::all()
        .filter(|command| *command != ShellCommand::CommandPalette)
        .map(|command| PaletteEntry {
            id: command.id().to_owned(),
            title: command.label().to_owned(),
            shortcut: keymap
                .chord_of(command)
                .and_then(Chord::parse)
                .map(|chord| chord.label(Primary::native())),
        })
        .collect()
}

impl ShellView {
    pub(super) fn show_pairing(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.pairing.is_some() {
            return;
        }
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let session = self.data.pairing(signal);
        let dialog = cx.new(|cx| PairingView::new(session, window, cx));
        self.subscriptions
            .push(cx.subscribe(&dialog, Self::on_pairing));
        self.pairing = Some(dialog);
        self.update_overlay(cx);
    }

    #[expect(
        clippy::trivially_copy_pass_by_ref,
        reason = "gpui subscribe passes the event by reference"
    )]
    fn on_pairing(
        &mut self,
        _: Entity<PairingView>,
        event: &PairingEvent,
        cx: &mut Context<'_, Self>,
    ) {
        if matches!(event, PairingEvent::Close) {
            self.pairing = None;
            self.update_overlay(cx);
        }
    }

    pub(super) fn show_branch_import(
        &mut self,
        row: &str,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.branch_import.is_some() {
            return;
        }
        let (signal, _) = crate::data_signal::callbacks(&self.requests, &self.data_queued);
        let opened = self
            .data
            .branch_import(&sidebar::RowId::from(row), signal)
            .inspect_err(|error| tracing::warn!(%error, "the branch import could not open"));
        let Ok(import) = opened else {
            return;
        };
        let dialog = cx.new(|cx| BranchImportView::new(import, window, cx));
        self.subscriptions
            .push(cx.subscribe(&dialog, Self::on_branch_import));
        self.branch_import = Some(dialog);
        self.update_overlay(cx);
    }

    #[expect(
        clippy::trivially_copy_pass_by_ref,
        reason = "gpui subscribe passes the event by reference"
    )]
    fn on_branch_import(
        &mut self,
        _: Entity<BranchImportView>,
        _: &ImportEvent,
        cx: &mut Context<'_, Self>,
    ) {
        self.branch_import = None;
        self.update_overlay(cx);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_palette_lists_every_command_but_itself_with_its_shortcut() {
        let entries = palette_entries(&shell_keys::Keymap::defaults());
        assert!(
            entries
                .iter()
                .all(|entry| entry.id != ShellCommand::CommandPalette.id())
        );
        let close = entries.iter().find(|entry| entry.id == "tabs.closeActive");
        assert_eq!(
            close.and_then(|entry| entry.shortcut.as_deref()),
            Some("Ctrl+W")
        );
    }

    #[test]
    fn a_typed_key_becomes_a_chord() {
        let stroke = gpui::Keystroke::parse("ctrl-shift-t").unwrap();
        assert_eq!(
            keystroke_of(&stroke).chord().as_deref(),
            Some("mod+shift+t")
        );
    }
}
