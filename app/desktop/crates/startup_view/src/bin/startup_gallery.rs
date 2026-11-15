#![allow(
    clippy::print_stderr,
    reason = "the example reports startup failures on the terminal"
)]
//! Shows the startup view in one phase, or plays a whole start-up.
//!
//! Usage: `startup_gallery [--phase starting|migrating|halted|restarting|unresponsive|gave-up|stopped|demo]
//! [--theme crowbar|frosted] [--mode light|dark] [--size WxH]`.

use std::rc::Rc;
use std::time::Duration;

use gpui::{
    App, AppContext as _, Application, AsyncApp, Bounds, Context, Entity, WindowBounds,
    WindowOptions, px, size,
};
use startup_view::{
    MigrationStep, MonotonicClock, Outage, StartupActions, StartupCommand, StartupEvent,
    StartupInput, StartupTiming, StartupView,
};
use ui::{Theme, ThemeId, ThemeMode};

const DEFAULT_SIZE: (f32, f32) = (1200.0, 800.0);
const STEPS: u32 = 5;
const STEP_TIME: Duration = Duration::from_millis(1200);
const REPORT: &str = concat!(
    "crowbar 0.1.0 linux x86_64\n",
    "the daemon exited 3 times in 60s\n\n",
    "--- log ---\n",
    "2026-11-14T09:12:01Z INFO  opening home\n",
    "2026-11-14T09:12:01Z ERROR migrate step 3 failed: no such table: spaces\n",
    "2026-11-14T09:12:02Z INFO  shutting down",
);
const HALT_MESSAGE: &str = "migrate: step 3 \"Convert workspace settings\": no such table: spaces";
const RESTART_ATTEMPT: u32 = 2;
const MIGRATING_PHASE_STEP: u32 = 2;

struct Actions {
    quits_on: StartupCommand,
}

impl StartupActions for Actions {
    fn perform(
        &self,
        command: StartupCommand,
        cx: &mut App,
    ) {
        if command == self.quits_on {
            cx.quit();
        }
    }
}

fn flag(name: &str) -> Option<String> {
    let args: Vec<String> = std::env::args().collect();
    let position = args
        .iter()
        .position(|arg| arg.strip_prefix("--") == Some(name))?;
    args.get(position + 1).cloned()
}

fn parse_size(value: &str) -> Option<(f32, f32)> {
    let (width, height) = value.split_once('x')?;
    Some((width.parse().ok()?, height.parse().ok()?))
}

fn migrating(index: u32) -> StartupInput {
    let names = [
        "Move chats to the new layout",
        "Rebuild the search index",
        "Convert workspace settings",
        "Backfill review threads",
        "Verify the result",
    ];
    let name = names
        .get(usize::try_from(index.saturating_sub(1)).unwrap_or(0))
        .copied()
        .unwrap_or("Finish");
    StartupInput::Migrating {
        step: Some(MigrationStep::new(name, index, STEPS)),
    }
}

fn halted() -> StartupInput {
    StartupInput::Halted {
        reason: Some("step_failed".to_owned()),
        message: HALT_MESSAGE.to_owned(),
    }
}

fn restarting() -> StartupInput {
    StartupInput::Unavailable(Outage::Recovering {
        attempt: RESTART_ATTEMPT,
        reason: "the daemon exited with status 1".to_owned(),
    })
}

fn phase_input(phase: &str) -> Vec<StartupInput> {
    match phase {
        "starting" => vec![StartupInput::Starting],
        "migrating" => vec![migrating(MIGRATING_PHASE_STEP)],
        "halted" => vec![halted()],
        "restarting" => vec![restarting()],
        "unresponsive" => vec![StartupInput::Unavailable(Outage::Unresponsive)],
        "gave-up" => vec![StartupInput::Unavailable(Outage::GaveUp {
            report: REPORT.to_owned(),
        })],
        "stopped" => vec![StartupInput::Unavailable(Outage::Stopped)],
        _ => Vec::new(),
    }
}

fn play(
    view: &Entity<StartupView>,
    cx: &mut App,
) {
    let view = view.clone();
    cx.spawn(async move |cx| play_sequence(&view, cx).await)
        .detach();
}

async fn play_sequence(
    view: &Entity<StartupView>,
    cx: &mut AsyncApp,
) {
    let sequence = std::iter::once(StartupInput::Starting)
        .chain((1..=STEPS).map(migrating))
        .chain(std::iter::once(StartupInput::Ready));
    for input in sequence {
        cx.background_executor().timer(STEP_TIME).await;
        view.update(cx, |view, cx| view.apply(input, cx)).ok();
    }
}

fn timing_for(phase: &str) -> StartupTiming {
    if phase == "demo" {
        return StartupTiming::default();
    }
    StartupTiming::new(Duration::ZERO, Duration::ZERO)
}

fn build(
    phase: &str,
    cx: &mut Context<'_, StartupView>,
) -> StartupView {
    let timing = timing_for(phase);
    let mut view = StartupView::new(
        timing,
        Rc::new(MonotonicClock::start()),
        Rc::new(Actions {
            quits_on: StartupCommand::Quit,
        }),
        cx,
    );
    for input in phase_input(phase) {
        view.apply(input, cx);
    }
    view
}

#[expect(
    clippy::trivially_copy_pass_by_ref,
    reason = "gpui hands subscribers the event by reference"
)]
fn quit_when_handed_over(
    _view: Entity<StartupView>,
    event: &StartupEvent,
    cx: &mut App,
) {
    if *event == StartupEvent::HandedOver {
        cx.quit();
    }
}

fn open(
    cx: &mut App,
    phase: &str,
) {
    let (width, height) = flag("size")
        .as_deref()
        .and_then(parse_size)
        .unwrap_or(DEFAULT_SIZE);
    let bounds = Bounds::centered(None, size(px(width), px(height)), cx);
    let options = WindowOptions {
        window_bounds: Some(WindowBounds::Windowed(bounds)),
        ..WindowOptions::default()
    };
    let opened = cx.open_window(options, |_, cx| cx.new(|cx| build(phase, cx)));
    let Ok(view) = opened.and_then(|window| window.entity(cx)) else {
        eprintln!("startup_gallery: cannot open the window");
        cx.quit();
        return;
    };
    cx.subscribe(&view, quit_when_handed_over).detach();
    if phase == "demo" {
        play(&view, cx);
    }
}

fn main() {
    let phase = flag("phase").unwrap_or_else(|| "demo".to_owned());
    let id = match flag("theme").as_deref() {
        Some("frosted") => ThemeId::Frosted,
        _ => ThemeId::Crowbar,
    };
    let mode = match flag("mode").as_deref() {
        Some("dark") => ThemeMode::Dark,
        _ => ThemeMode::Light,
    };
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, &phase, id, mode));
}

fn start(
    cx: &mut App,
    phase: &str,
    id: ThemeId,
    mode: ThemeMode,
) {
    if let Err(error) = ui::init(cx) {
        eprintln!("startup_gallery: {error}");
        cx.quit();
        return;
    }
    Theme::of(cx).with_id(id).with_mode(mode).install(cx);
    open(cx, phase);
    cx.activate(true);
}
