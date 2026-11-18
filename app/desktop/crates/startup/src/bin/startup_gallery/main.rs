//! Shows the startup view in one phase, or plays a whole start-up.
//!
//! Usage: `startup_gallery [--phase starting|migrating|halted|restarting|unresponsive|gave-up|stopped|demo]
//! [--theme crowbar|frosted] [--mode light|dark] [--size WxH]`.

mod phases;

use std::cell::RefCell;
use std::rc::Rc;
use std::time::Duration;

use gpui::{
    App, AppContext as _, Application, AsyncApp, Bounds, Context, Entity, WindowBounds,
    WindowOptions, px, size,
};
use startup::{
    MonotonicClock, StartupActions, StartupCommand, StartupEvent, StartupInput, StartupView,
};
use ui::{Theme, ThemeId, ThemeMode};

use crate::phases::{DEMO_PHASE, STEPS, migrating, phase_input, timing_for};

const DEFAULT_SIZE: (f32, f32) = (1200.0, 800.0);
const STEP_TIME: Duration = Duration::from_millis(1200);

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
    play_all(view, sequence, cx).await;
}

async fn play_all(
    view: &Entity<StartupView>,
    sequence: impl Iterator<Item = StartupInput>,
    cx: &mut AsyncApp,
) -> Option<()> {
    for input in sequence {
        step(view, input, cx).await?;
    }
    Some(())
}

async fn step(
    view: &Entity<StartupView>,
    input: StartupInput,
    cx: &mut AsyncApp,
) -> Option<()> {
    cx.background_executor().timer(STEP_TIME).await;
    view.update(cx, |view, cx| view.apply(input, cx)).ok()
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

fn quit_when_handed_over(
    event: StartupEvent,
    cx: &mut App,
) {
    if event == StartupEvent::HandedOver {
        cx.quit();
    }
}

#[derive(Debug, thiserror::Error)]
enum Failure {
    #[error("cannot open the window")]
    OpenWindow,
    #[error("init: {0}")]
    Init(#[from] ui::Error),
}

type FailureSlot = Rc<RefCell<Option<Failure>>>;

fn open(
    cx: &mut App,
    phase: &str,
    failure: &FailureSlot,
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
        *failure.borrow_mut() = Some(Failure::OpenWindow);
        cx.quit();
        return;
    };
    cx.subscribe(&view, |_view, event: &StartupEvent, cx| {
        quit_when_handed_over(*event, cx);
    })
    .detach();
    if phase == DEMO_PHASE {
        play(&view, cx);
    }
}

fn main() -> Result<(), Failure> {
    let phase = flag("phase").unwrap_or_else(|| DEMO_PHASE.to_owned());
    let id = match flag("theme").as_deref() {
        Some("frosted") => ThemeId::Frosted,
        _ => ThemeId::Crowbar,
    };
    let mode = match flag("mode").as_deref() {
        Some("dark") => ThemeMode::Dark,
        _ => ThemeMode::Light,
    };
    let failure = FailureSlot::default();
    let slot = Rc::clone(&failure);
    Application::new()
        .with_assets(ui::assets())
        .run(move |cx: &mut App| start(cx, &phase, &Look { id, mode }, &slot));
    failure.take().map_or(Ok(()), Err)
}

struct Look {
    id: ThemeId,
    mode: ThemeMode,
}

fn start(
    cx: &mut App,
    phase: &str,
    look: &Look,
    failure: &FailureSlot,
) {
    if let Err(error) = ui::init(cx) {
        *failure.borrow_mut() = Some(Failure::from(error));
        cx.quit();
        return;
    }
    Theme::of(cx)
        .with_id(look.id)
        .with_mode(look.mode)
        .install(cx);
    open(cx, phase, failure);
    cx.activate(true);
}
