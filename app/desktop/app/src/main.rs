//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod adapters;
mod app_options;
mod daemon;
mod error;
mod legacy_drafts;
mod legacy_settings;
mod native;
mod panes;
mod shell;
#[cfg(test)]
mod test_support;

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
use crate::panes::kinds::host_builder::build_host;
use crate::shell::inbox::data_signal;
use crate::shell::ports::Ports;
use crate::shell::shell_state::ShellState;
use crate::shell::shell_state::active_space;
use crate::shell::shell_state::persistence::layout_store::LayoutStore;
use crate::shell::shell_view::ShellView;
use crate::shell::shell_view::assembly::shell_parts::ShellParts;
use crate::shell::window::app_menu;
use crate::shell::window::caret_follower;
use crate::shell::window::last_window;
use crate::shell::window::window_memory::WindowMemory;
use crate::shell::window::window_vibrancy;

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
        window.set_app_id(shell::window::window_memory::APP_ID);
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
