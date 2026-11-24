//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod adapters;
mod agent_requests;
mod app_menu;
mod app_options;
mod backend;
mod browser_bars;
mod browser_homepage;
mod browser_ids;
mod browser_pages;
mod browser_tabs;
mod byte_source;
mod caret_follower;
mod chat_hosts;
mod chat_label;
mod chat_locks;
mod chat_workspaces;
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
mod daemon_peer;
mod data_signal;
mod diagnostics_export;
mod direction;
mod dirty_panes;
mod document_request;
mod document_source;
mod dropped_files;
mod editor_panes;
mod editor_preferences;
mod error;
mod event_commands;
mod family_names;
mod file_prompt;
mod file_reply;
mod file_target;
mod focus_neighbour;
mod focused_chat;
mod frame_clock;
mod frame_stats;
mod group_focus;
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
mod legacy_drafts;
mod legacy_settings;
mod link_choice;
mod link_menu;
mod link_state;
mod loaded_layout;
mod logged;
mod name_event;
mod name_prompt;
mod native_panes;
mod new_tab_action;
mod new_tab_panes;
mod new_tab_view;
mod notice_key;
mod notices;
mod open_all;
mod open_chats;
mod outcome;
mod page_shot;
mod pane_breakpoint;
mod pane_opener;
mod pane_settings;
mod pane_spec;
mod pane_theme;
mod pane_toast_change;
mod peer_line;
mod pending_close;
mod pending_file;
mod pending_layout;
mod pending_loads;
mod phase_input;
mod picking_panes;
mod pixels;
mod ports;
mod preferences_store;
mod prefill;
mod project_slot;
mod prompt_event;
mod prompt_words;
mod readout_dock;
mod readout_lines;
mod readout_net;
mod readout_snapshot;
mod readout_state;
mod readout_view;
mod reconnect_count;
mod request_queue;
mod reveal_target;
mod review_panes;
mod save_health;
mod save_refusal;
mod save_report;
mod saved_file;
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
mod terminal_root;
#[cfg(test)]
mod test_support;
mod titles;
mod traffic_lights;
mod transport_book;
mod unsaved_dialog;
mod view_life;
mod window_memory;
mod window_title;
mod window_vibrancy;
mod workspace;
mod workspace_event;

use std::path::{Path, PathBuf};
use std::rc::Rc;
use std::sync::Arc;

use diagnostics::LogPaths;
use gpui::{App, AppContext as _, Entity, Window};

use crate::adapters::disk_files::DiskFiles;
use crate::adapters::disk_preferences::DiskPreferences;
use crate::adapters::system_clock::SystemClock;
use crate::app_options::AppOptions;
use crate::daemon_link::DaemonLink;
use crate::host_builder::build_host;
use crate::layout_store::LayoutStore;
use crate::ports::Ports;
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
    editorview::init(cx);
    cx.bind_keys(settingsview::bindings());
    window_vibrancy::install(window_vibrancy::of_platform(), cx);
    app_menu::install(cx);
    let memory = WindowMemory::new(&options.state_dir().join("window.json"));
    let window_options = memory.options(cx);
    let opened = cx.open_window(window_options, |window, cx| {
        let view = cx.new(|cx| ShellView::new(parts, window, cx));
        remember_close(&memory, window, cx, &view);
        view
    });
    let main = match opened {
        Ok(main) => main,
        Err(error) => return quit_after(cx, &error, "could not open the window"),
    };
    if let Err(error) = ui::TopLayer::open(main.into(), cx) {
        return quit_after(cx, &error, "could not open the window's top layer");
    }
    cx.activate(true);
}

fn system_ports(preferences_path: PathBuf) -> Ports {
    let preferences = DiskPreferences {
        path: preferences_path,
    };
    Ports {
        clock: Arc::new(SystemClock::default()),
        bytes: Arc::new(DiskFiles::default()),
        preferences: Arc::new(preferences),
    }
}

fn main() -> anyhow::Result<()> {
    #[cfg(target_os = "linux")]
    platform::force_x11();
    let options = AppOptions::from_env(|key| std::env::var(key).ok());
    diagnostics::start(
        &LogPaths::in_data_dir(&options.state_dir()),
        env!("CARGO_PKG_VERSION"),
    )?;
    legacy_settings::import_old_settings(std::env::var_os("HOME").map(PathBuf::from), &options);
    legacy_drafts::import_old_drafts(std::env::var_os("HOME").map(PathBuf::from), &options);
    let link = Rc::new(DaemonLink::start(&options)?);
    let (caret, caret_changes) = caret_follower::channel();
    let host = build_host(&options, (link.transport(), caret), Path::is_file)?;
    let store = LayoutStore::on_disk(options.state_dir().join("layout.json"));
    let state = ShellState::new(host, store)?;
    let preferences_path = options.state_dir().join("sidebar.json");
    let shutdown = Rc::clone(&link);
    let parts = ShellParts {
        state,
        link,
        ports: system_ports(preferences_path.clone()),
        preferences_path,
    };
    gpui_platform::application()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| {
            caret_follower::follow(caret_changes, cx);
            start(cx, (&options, parts));
        });
    shutdown.shutdown();
    Ok(())
}
