//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod active_space;
mod adapters;
mod agent_requests;
mod app_menu;
mod app_options;
mod browser_homepage;
mod byte_source;
mod caret_follower;
mod chrome;
mod clock;
mod close_choice;
mod close_step;
mod closing;
mod daemon;
mod data_signal;
mod direction;
mod editor_preferences;
mod error;
mod event_commands;
mod family_names;
mod file_prompt;
mod file_reply;
mod focus_neighbour;
mod frame_clock;
mod frame_stats;
mod hosts;
mod installed_fonts;
mod landing;
mod last_window;
mod layout_files;
mod layout_store;
mod layout_writer;
mod legacy_drafts;
mod legacy_settings;
mod loaded_layout;
mod logged;
mod name_event;
mod name_prompt;
#[cfg(target_os = "linux")]
mod native;
mod notice_key;
mod notices;
mod open_all;
mod outcome;
mod pane_settings;
mod panes;
mod pending_close;
mod pending_file;
mod pending_layout;
mod pending_loads;
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
mod reload_dialog;
mod request_queue;
mod reveal_target;
mod save_health;
mod save_report;
mod search_engine_setting;
mod settings_modal;
mod shell_parts;
mod shell_request;
mod shell_state;
mod shell_view;
mod sidebar_action;
mod sidebar_requests;
mod sidebar_surface;
mod startup_requests;
mod terminal_preferences;
#[cfg(test)]
mod test_support;
mod traffic_lights;
mod unsaved_dialog;
mod view_life;
mod window_memory;
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
use crate::daemon::daemon_link::DaemonLink;
use crate::layout_store::LayoutStore;
use crate::panes::kinds::host_builder::build_host;
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
    #[cfg(target_os = "linux")]
    native::install(parts.state.host(), cx);
    #[cfg(target_os = "linux")]
    native::float::install(cx);
    #[cfg(target_os = "linux")]
    adapters::key_places::install(cx);
    app_menu::install(cx);
    last_window::quit_with_last_window(cx, |cx| cx.quit());
    let memory = WindowMemory::new(&options.state_dir().join("window.json"));
    let window_options = memory.options(cx);
    let opened = cx.open_window(window_options, |window, cx| {
        // PLATFORM: linux X11 windows take WM_CLASS only from set_app_id; wayland reads the options.
        window.set_app_id(window_memory::APP_ID);
        #[cfg(target_os = "linux")]
        adapters::window_activity::follow(window, cx);
        #[cfg(target_os = "linux")]
        adapters::drop_relay::install(window, cx);
        let view = cx.new(|cx| ShellView::new(parts, window, cx));
        remember_close(&memory, window, cx, &view);
        view
    });
    if let Err(error) = opened {
        return quit_after(cx, &error, "could not open the window");
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
    let (requests, queue) = async_channel::unbounded();
    let (data, data_queued) = data_signal::start_data(&link, &requests);
    let state = ShellState::new(host, store, active_space::of_store(&data))?;
    let preferences_path = options.state_dir().join("sidebar.json");
    let shutdown = Rc::clone(&link);
    let parts = ShellParts {
        state,
        link,
        ports: system_ports(preferences_path.clone()),
        preferences_path,
        data,
        data_queued,
        requests,
        queue,
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
