//! A terminal view in a test window wired to a fake daemon peer.

use std::cell::RefCell;
use std::rc::Rc;

use gpui::{Entity, TestAppContext, VisualTestContext, point, px};
use terminal::{ClientMessage, DaemonFrame};

use crate::daemon::Link;
use crate::daemon::Peer;
use crate::settings::Settings;
use crate::terminal_view::TerminalView;

const LEFT_OF_CENTER: f32 = 0.25;
const HALF_CELL: f32 = 0.5;

pub(crate) struct Harness {
    pub(crate) view: Entity<TerminalView>,
    pub(crate) peer: Peer,
}

pub(crate) fn open(cx: &mut TestAppContext) -> (Harness, &mut VisualTestContext) {
    cx.update(|cx| ui::init(cx).unwrap_or_else(|error| panic!("{error}")));
    let (link, peer) = Link::open();
    let (view, cx) =
        cx.add_window_view(|window, cx| TerminalView::new(link, Settings::default(), window, cx));
    cx.update(|window, _| window.activate_window());
    draw(cx);
    (Harness { view, peer }, cx)
}

pub(crate) fn draw(cx: &mut VisualTestContext) {
    cx.run_until_parked();
    cx.update(|window, cx| window.draw(cx).clear(cx));
    cx.run_until_parked();
}

pub(crate) fn feed(
    harness: &Harness,
    cx: &mut VisualTestContext,
    text: &str,
) {
    harness
        .peer
        .deliver_blocking(DaemonFrame::Output(text.as_bytes().to_vec()));
    draw(cx);
}

pub(crate) fn sent(harness: &Harness) -> Vec<ClientMessage> {
    std::iter::from_fn(|| harness.peer.try_next_message()).collect()
}

pub(crate) fn typed(harness: &Harness) -> String {
    sent(harness)
        .into_iter()
        .filter_map(|message| match message {
            ClientMessage::Input(text) => Some(text),
            _ => None,
        })
        .collect()
}

pub(crate) fn first_row(
    harness: &Harness,
    cx: &mut VisualTestContext,
) -> String {
    let snapshot = harness
        .view
        .read_with(cx, |view, _| view.terminal.snapshot());
    snapshot.rows.first().map_or_else(String::new, |row| {
        row.segments
            .iter()
            .map(|segment| segment.text.as_str())
            .collect()
    })
}

pub(crate) fn cell_position(
    harness: &Harness,
    cx: &mut VisualTestContext,
    column: usize,
) -> gpui::Point<gpui::Pixels> {
    let layout = harness.view.read_with(cx, |view, _| view.layout);
    let Some(layout) = layout else {
        return point(px(0.0), px(0.0));
    };
    let metrics = layout.metrics;
    layout.origin
        + point(
            px(metrics.cell_width * (terminal::cells_f32(column) + LEFT_OF_CENTER)),
            px(metrics.line_height * HALF_CELL),
        )
}

pub(crate) fn focus(
    harness: &Harness,
    cx: &mut VisualTestContext,
) {
    cx.update(|window, cx| harness.view.update(cx, |view, cx| view.focus(window, cx)));
    draw(cx);
}

pub(crate) fn record_events<T, E>(
    cx: &mut VisualTestContext,
    entity: &Entity<T>,
) -> Rc<RefCell<Vec<E>>>
where
    T: gpui::EventEmitter<E> + 'static,
    E: Clone + 'static,
{
    let events = Rc::new(RefCell::new(Vec::new()));
    let seen = Rc::clone(&events);
    cx.update(|_, cx| subscribe_into(cx, entity, seen));
    events
}

fn subscribe_into<T, E>(
    cx: &mut gpui::App,
    entity: &Entity<T>,
    seen: Rc<RefCell<Vec<E>>>,
) where
    T: gpui::EventEmitter<E> + 'static,
    E: Clone + 'static,
{
    cx.subscribe(entity, move |_, event: &E, _| record(&seen, event))
        .detach();
}

fn record<E: Clone>(
    seen: &RefCell<Vec<E>>,
    event: &E,
) {
    seen.borrow_mut().push(event.clone());
}
