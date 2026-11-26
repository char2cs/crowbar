use gpui::TestAppContext;
use keymap::ShellCommand;
use protocol::PaneId;
use terminalview::Event;

use super::ShellView;
use crate::panes::kinds::core_panes::TERMINAL;
use crate::shell::shell_view::tests::{Opened, open_shell};

pub(super) fn with_terminal(cx: &mut TestAppContext) -> (Opened, PaneId) {
    let mut opened = open_shell(cx);
    let shell = opened.shell.clone();
    let terminal = shell.update_in(&mut opened.window, |shell, window, cx| {
        shell.new_terminal(window, cx);
        panes_of(shell, TERMINAL).remove(0)
    });
    (opened, terminal)
}

pub(super) fn panes_of(
    shell: &ShellView,
    type_id: &str,
) -> Vec<PaneId> {
    let layout = shell.state.layout();
    let is_kind = |pane: &PaneId| {
        layout
            .pane(pane)
            .is_some_and(|instance| instance.type_id.as_str() == type_id)
    };
    let tab = shell.state.showing().map(layout::Tab::pane_ids);
    tab.unwrap_or_default()
        .into_iter()
        .filter(is_kind)
        .collect()
}

pub(super) fn tell(
    opened: &mut Opened,
    (pane, event): (&PaneId, Event),
) {
    let shell = opened.shell.clone();
    shell.update_in(&mut opened.window, |shell, window, cx| {
        shell.on_terminal((pane, event), window, cx);
    });
    opened.window.run_until_parked();
}

/// The keystrokes of `command`'s default chord, with `mod` as gpui's `secondary` so each platform
/// presses its own primary modifier.
pub(super) fn keys_of(command: ShellCommand) -> String {
    let chord = command.default_chord();
    let (modifiers, key) = chord.rsplit_once('+').unwrap_or(("", chord));
    modifiers
        .split('+')
        .filter(|modifier| !modifier.is_empty())
        .map(|modifier| match modifier {
            "mod" => "secondary",
            other => other,
        })
        .chain([key])
        .collect::<Vec<_>>()
        .join("-")
}

pub(super) fn notice_count(opened: &mut Opened) -> usize {
    opened.window.run_until_parked();
    opened
        .shell
        .read_with(&opened.window, |shell, cx| shell.notices.read(cx).len())
}

pub(super) fn saves_asked(mock: &webview::MockPlatform) -> usize {
    let asks = |call: &&webview::Call| matches!(call, webview::Call::Eval { script, .. } if script.contains("pane.save"));
    mock.calls().iter().filter(asks).count()
}

pub(super) fn settled_notice_count(opened: &mut Opened) -> usize {
    opened.window.executor().advance_clock(ui::timing::SLOW);
    notice_count(opened)
}

pub(super) fn open_file(
    opened: &mut Opened,
    path: &str,
) -> PaneId {
    opened.shell.update(&mut opened.window, |shell, cx| {
        shell.open_workspace_file(path, cx);
        let focused = shell.state.showing().and_then(layout::Tab::focused);
        focused.cloned().expect("the file pane has focus")
    })
}
