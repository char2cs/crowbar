//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod adapters;
mod app_menu;
mod app_options;
mod browser_bars;
mod browser_homepage;
mod browser_ids;
mod chat_label;
mod chat_route;
mod chat_terminals;
mod chrome;
mod clock;
mod close_choice;
mod close_guard;
mod close_step;
mod closed_panes;
mod closing;
mod core_panes;
mod core_type;
mod daemon_link;
mod daemon_notice;
mod data_signal;
mod diagnostics_export;
mod direction;
mod dirty_panes;
mod dropped_files;
mod env_change;
mod error;
mod event_commands;
mod family_names;
mod file_prompt;
mod file_reply;
mod focus_neighbour;
mod focused_chat;
mod frame_clock;
mod frame_stats;
mod frozen_layer;
mod holes;
mod host_builder;
mod hosts;
mod hover_tracker;
mod installed_fonts;
mod landing;
mod launched;
mod layout_files;
mod layout_store;
mod layout_writer;
mod legacy_settings;
mod link_state;
mod loaded_layout;
mod new_tab_action;
mod new_tab_panes;
mod new_tab_view;
mod notice_key;
mod notices;
mod open_all;
mod outcome;
mod overlay_requests;
mod pane_opener;
mod pane_settings;
mod pane_spec;
mod pane_theme;
mod pending_close;
mod pending_file;
mod pending_layout;
mod phase_input;
mod picking_panes;
mod pixels;
mod plan;
mod project_slot;
mod prompt_event;
mod readout_dock;
mod readout_lines;
mod readout_net;
mod readout_snapshot;
mod readout_state;
mod readout_view;
mod reconnect_count;
mod request_queue;
mod reveal_target;
mod save_health;
mod save_report;
mod search_engine_setting;
mod serials;
mod settings_modal;
mod shell_pane_source;
mod shell_parts;
mod shell_request;
mod shell_state;
mod shell_view;
mod sidebar_action;
mod sidebar_requests;
mod sidebar_surface;
mod single_precision;
mod stack_policy;
mod startup_action;
mod startup_command;
mod startup_requests;
mod tab_cap;
mod tab_looks;
mod tab_scope;
mod terminal_ids;
mod terminal_owner;
mod terminal_panes;
mod terminal_preferences;
#[cfg(test)]
mod test_support;
mod titles;
mod transport_book;
mod unsaved_dialog;
mod window_memory;
mod window_title;
mod window_vibrancy;
mod workspace;
mod workspace_event;
mod x11_session;

use std::path::PathBuf;
use std::rc::Rc;
use std::sync::Arc;

use gpui::{App, AppContext as _, Application, Entity, Window};
use shell_diagnostics::LogPaths;

use crate::adapters::system_clock::SystemClock;
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

fn remember_close(
    memory: &WindowMemory,
    window: &Window,
    cx: &App,
    view: &Entity<ShellView>,
) {
    let guard = view.downgrade();
    memory.remember_on_close(window, cx, move |_, cx| {
        guard
            .update(cx, ShellView::allow_window_close)
            .unwrap_or(true)
    });
}

fn start(
    cx: &mut App,
    (options, parts): (&AppOptions, ShellParts),
) {
    if let Err(error) = ui::init(cx) {
        return quit_after(cx, &error, "could not install the design system");
    }
    window_vibrancy::install(window_vibrancy::of_platform(), cx);
    app_menu::install(cx);
    let memory = WindowMemory::new(&options.state_dir().join("window.json"));
    let window_options = memory.options(cx);
    let opened = cx.open_window(window_options, |window, cx| {
        let view = cx.new(|cx| ShellView::new(parts, window, cx));
        remember_close(&memory, window, cx, &view);
        view
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
    legacy_settings::import_old_settings(std::env::var_os("HOME").map(PathBuf::from), &options);
    let link = Rc::new(DaemonLink::start(&options)?);
    let host = build_host(&options, link.transport())?;
    let store = LayoutStore::on_disk(options.state_dir().join("layout.json"));
    let state = ShellState::new(host, store)?;
    let preferences_path = options.state_dir().join("sidebar.json");
    let shutdown = Rc::clone(&link);
    let parts = ShellParts {
        state,
        link,
        preferences_path,
        clock: Arc::new(SystemClock::default()),
    };
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, (&options, parts)));
    shutdown.shutdown();
    Ok(())
}
