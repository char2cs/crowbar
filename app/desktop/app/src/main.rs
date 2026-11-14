//! The Crowbar desktop shell: a GPUI window whose content area hosts panes as web views.

mod app_options;
mod core_panes;
mod event_commands;
mod loopback_daemon;
mod shell_state;
mod shell_view;

use std::sync::Arc;

use gpui::{App, Application, Bounds, WindowBounds, WindowOptions, prelude::*, px, size};
use tracing_subscriber::EnvFilter;

use crate::app_options::AppOptions;
use crate::loopback_daemon::LoopbackDaemon;
use crate::shell_state::ShellState;
use crate::shell_view::ShellView;

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")))
        .init();
    let options = AppOptions::from_env(|key| std::env::var(key).ok());
    let state = ShellState::new(&options, Arc::new(LoopbackDaemon))?;
    Application::new().run(move |cx: &mut App| {
        let bounds = Bounds::centered(None, size(px(1280.0), px(800.0)), cx);
        let opened = cx.open_window(
            WindowOptions {
                window_bounds: Some(WindowBounds::Windowed(bounds)),
                ..WindowOptions::default()
            },
            |_, cx| cx.new(|cx| ShellView::new(state, cx)),
        );
        if let Err(error) = opened {
            tracing::error!(%error, "could not open the window");
            cx.quit();
        }
        cx.activate(true);
    });
    Ok(())
}
