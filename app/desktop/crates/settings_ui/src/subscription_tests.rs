use std::cell::Cell;
use std::rc::Rc;

use super::*;

#[test]
fn dropping_cancels_exactly_once() {
    let cancelled = Rc::new(Cell::new(0));
    let sink = Rc::clone(&cancelled);
    let subscription = Subscription::new(move || sink.set(sink.get() + 1));
    assert_eq!(cancelled.get(), 0, "alive subscriptions stay registered");
    drop(subscription);
    assert_eq!(cancelled.get(), 1, "drop cancels once");
}
