//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod app_options;
mod chat_label;
mod chrome;
mod core_panes;
mod daemon_link;
mod error;
mod event_commands;
mod holes;
mod host_builder;
mod layout_store;
mod pane_opener;
mod pane_source;
mod phase_input;
mod pixels;
mod request_sinks;
mod serials;
mod settings_modal;
mod shell_outcome;
mod shell_request;
mod shell_state;
mod shell_view;
mod sidebar_action;
mod startup_command;
mod titles;
mod workspace;

use std::rc::Rc;

use gpui::{App, AppContext as _, Application, Bounds, WindowBounds, WindowOptions, px, size};
use tracing_subscriber::EnvFilter;

use crate::app_options::AppOptions;
use crate::daemon_link::DaemonLink;
use crate::host_builder::build_host;
use crate::layout_store::LayoutStore;
use crate::shell_state::ShellState;
use crate::shell_view::{ShellParts, ShellView};

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();
    let options = AppOptions::from_env(|key| std::env::var(key).ok());
    let link = Rc::new(DaemonLink::start(&options)?);
    let host = build_host(&options, link.transport())?;
    let store = LayoutStore::new(options.state_dir().join("layout.json"));
    let state = ShellState::new(host, store)?;
    let preferences_path = options.state_dir().join("sidebar.json");
    Application::new()
        .with_assets(ui::Assets)
        .run(move |cx: &mut App| {
            if let Err(error) = ui::init(cx) {
                tracing::error!(%error, "could not install the design system");
                cx.quit();
                return;
            }
            let bounds = Bounds::centered(None, size(px(1280.0), px(800.0)), cx);
            let parts = ShellParts {
                state,
                link,
                preferences_path,
            };
            let opened = cx.open_window(
                WindowOptions {
                    window_bounds: Some(WindowBounds::Windowed(bounds)),
                    ..WindowOptions::default()
                },
                |window, cx| cx.new(|cx| ShellView::new(parts, window, cx)),
            );
            if let Err(error) = opened {
                tracing::error!(%error, "could not open the window");
                cx.quit();
            }
            cx.activate(true);
        });
    Ok(())
}
