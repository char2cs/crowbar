//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod app_menu;
mod app_options;
mod browser_bars;
mod browser_ids;
mod chat_label;
mod chrome;
mod closed_panes;
mod core_panes;
mod core_type;
mod daemon_link;
mod daemon_notice;
mod data_signal;
mod diagnostics_export;
mod direction;
mod env_change;
mod error;
mod event_commands;
mod focus_neighbour;
mod focused_chat;
mod holes;
mod host_builder;
mod hover_tracker;
mod installed_fonts;
mod launched;
mod layout_store;
mod layout_writer;
mod link_state;
mod loaded_layout;
mod notice_key;
mod notices;
mod outcome;
mod overlay_requests;
mod pane_opener;
mod pane_spec;
mod pane_theme;
mod pending_layout;
mod phase_input;
mod pixels;
mod request_queue;
mod serials;
mod settings_modal;
mod shell_pane_source;
mod shell_parts;
mod shell_request;
mod shell_state;
mod shell_view;
mod sidebar_action;
mod sidebar_requests;
mod startup_action;
mod startup_command;
mod startup_requests;
#[cfg(test)]
mod test_support;
mod titles;
mod window_memory;
mod workspace;
mod workspace_event;
mod x11_session;

use std::rc::Rc;

use gpui::{App, AppContext as _, Application};
use shell_diagnostics::LogPaths;

use crate::app_options::AppOptions;
use crate::daemon_link::DaemonLink;
use crate::host_builder::build_host;
use crate::layout_store::LayoutStore;
use crate::shell_parts::ShellParts;
use crate::shell_state::ShellState;
use crate::shell_view::ShellView;
use crate::window_memory::WindowMemory;

fn quit_after(
    cx: &mut App,
    error: &dyn std::fmt::Display,
    what: &str,
) {
    tracing::error!(%error, "{what}");
    cx.quit();
}

fn start(
    cx: &mut App,
    (options, parts): (&AppOptions, ShellParts),
) {
    if let Err(error) = ui::init(cx) {
        return quit_after(cx, &error, "could not install the design system");
    }
    app_menu::install(cx);
    let memory = WindowMemory::new(&options.state_dir().join("window.json"));
    let window_options = memory.options(cx);
    let opened = cx.open_window(window_options, |window, cx| {
        memory.remember_on_close(window, cx);
        cx.new(|cx| ShellView::new(parts, window, cx))
    });
    if let Err(error) = opened {
        return quit_after(cx, &error, "could not open the window");
    }
    cx.activate(true);
}

fn main() -> anyhow::Result<()> {
    x11_session::force_x11();
    let options = AppOptions::from_env(|key| std::env::var(key).ok());
    shell_diagnostics::start(
        &LogPaths::in_data_dir(&options.state_dir()),
        env!("CARGO_PKG_VERSION"),
    )?;
    let link = Rc::new(DaemonLink::start(&options)?);
    let host = build_host(&options, link.transport())?;
    let store = LayoutStore::new(options.state_dir().join("layout.json"));
    let state = ShellState::new(host, store)?;
    let preferences_path = options.state_dir().join("sidebar.json");
    let shutdown = Rc::clone(&link);
    let parts = ShellParts {
        state,
        link,
        preferences_path,
    };
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, (&options, parts)));
    shutdown.shutdown();
    Ok(())
}
