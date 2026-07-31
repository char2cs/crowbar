#![forbid(unsafe_code)]

//! `crowbar-app` — the Crowbar native binary.
//!
//! Item 0.4: the first end-to-end proof that the vendored GPUI (§10.5), this
//! workspace and the daemon transport (§9.1) work together. It derives the
//! daemon's socket path, asks it `GET /v0/health`, and renders the answer in a
//! real GPUI window.
//!
//! Everything else — menus, panes, routing — is later phases.
//!
//! **This binary never touches the socket itself.** §4.2 makes `crowbar-client`
//! the only crate that talks to the daemon, and that stays true from the first
//! commit: a `UnixStream` here would be a hole in the layering that later items
//! would build on.

use crowbar_client::{Health, HealthError, Probe};
use gpui::{
    App, AppContext as _, Context, IntoElement, ParentElement as _, Render, SharedString,
    Styled as _, TitlebarOptions, Window, WindowOptions,
};
use gpui_component::{ActiveTheme as _, Root, v_flex};

fn main() {
    // Blocking, before the window exists, and on purpose. The daemon is a local
    // unix socket a few hundred microseconds away, this is a single request,
    // and the point of item 0.4 is to prove the round trip happened — a
    // "connecting…" state that resolves before the first frame would prove
    // less, not more. §9.1's real client moves this onto gpui's background
    // executor when there is a second caller to justify the machinery.
    let report = Report::probe();

    gpui_platform::application().run(move |cx: &mut App| {
        gpui_component::init(cx);

        let opened = cx.open_window(window_options(), |window, cx| {
            let view = cx.new(|_| report);
            cx.new(|cx| Root::new(view, window, cx).bg(cx.theme().background))
        });

        match opened {
            Ok(_) => cx.activate(true),
            Err(err) => {
                // No window means nothing can display the failure, so stderr is
                // the only channel left. Quitting beats sitting in a run loop
                // with no UI.
                eprintln!("crowbar-app: could not open a window: {err}");
                cx.quit();
            }
        }
    });
}

fn window_options() -> WindowOptions {
    WindowOptions {
        titlebar: Some(TitlebarOptions {
            title: Some(SharedString::new_static("Crowbar (native)")),
            ..TitlebarOptions::default()
        }),
        ..WindowOptions::default()
    }
}

/// What the window shows: the socket that was dialled and what came back.
///
/// Pre-rendered to strings at probe time rather than held as a
/// `Result<Health, HealthError>`, because `Render` runs on every frame and
/// formatting is not the view's job.
struct Report {
    lines: Vec<SharedString>,
}

impl Report {
    /// Derives the socket, probes the daemon, and turns either outcome into
    /// displayable lines. A daemon that is down is a normal outcome here — it
    /// is shown, not panicked on.
    fn probe() -> Self {
        match crowbar_client::probe_daemon() {
            Ok(probe) => Self::from_probe(&probe),
            Err(err) => Self {
                lines: vec![
                    "no daemon socket could be derived".into(),
                    err.to_string().into(),
                ],
            },
        }
    }

    fn from_probe(probe: &Probe) -> Self {
        let socket: SharedString = probe.socket.display().to_string().into();
        let mut lines = match &probe.result {
            Ok(health) => Self::describe(health),
            Err(err) => Self::describe_failure(err),
        };
        lines.push(socket);
        Self { lines }
    }

    fn describe(health: &Health) -> Vec<SharedString> {
        vec![
            "daemon reached".into(),
            format!("pid {}", health.pid).into(),
            format!("status {}", health.status).into(),
            format!("version {}", health.version).into(),
        ]
    }

    fn describe_failure(err: &HealthError) -> Vec<SharedString> {
        vec!["daemon unreachable".into(), err.to_string().into()]
    }
}

impl Render for Report {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        // Styling is deliberately the two theme tokens it takes to make text
        // legible. The sealed-token system (§4.3 rule 3, §6.1) does not exist
        // yet, and inventing a palette here would be work a later phase has to
        // unpick.
        v_flex()
            .size_full()
            .items_center()
            .justify_center()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .children(self.lines.iter().cloned())
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    /// Pins what the window shows, because the window itself cannot be read
    /// back: `screencapture` and the accessibility API are both permission-
    /// gated for an agent shell, so "what did it render" would otherwise be an
    /// unverifiable claim. `Render` consumes exactly `self.lines`, so fixing
    /// the lines fixes the frame.
    #[test]
    fn a_reachable_daemon_renders_pid_status_and_version() {
        let report = Report::from_probe(&Probe {
            socket: PathBuf::from("/tmp/crowbar-6d4f21ce150add3c.sock"),
            result: Ok(Health {
                pid: 62909,
                status: "ok".to_owned(),
                version: "0.1.0".to_owned(),
            }),
        });

        assert_eq!(
            report.lines,
            vec![
                SharedString::from("daemon reached"),
                SharedString::from("pid 62909"),
                SharedString::from("status ok"),
                SharedString::from("version 0.1.0"),
                SharedString::from("/tmp/crowbar-6d4f21ce150add3c.sock"),
            ],
        );
    }

    /// A down daemon is a rendered state, not a crash. The socket stays on the
    /// last line either way: when the probe fails it is the single most useful
    /// thing to show.
    #[test]
    fn an_unreachable_daemon_renders_the_reason_not_a_panic() {
        let socket = PathBuf::from("/tmp/crowbar-nothing-listening.sock");
        let err = crowbar_client::fetch_health(&socket).expect_err("nothing is bound there");
        let reason = err.to_string();

        let report = Report::from_probe(&Probe {
            socket: socket.clone(),
            result: Err(err),
        });

        assert_eq!(
            report.lines,
            vec![
                SharedString::from("daemon unreachable"),
                SharedString::from(reason),
                SharedString::from(socket.display().to_string()),
            ],
        );
    }

    #[test]
    fn the_window_is_titled() {
        assert_eq!(
            window_options()
                .titlebar
                .and_then(|titlebar| titlebar.title),
            Some(SharedString::new_static("Crowbar (native)")),
        );
    }
}
