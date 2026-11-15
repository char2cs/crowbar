//! Receives the emulator's events and keeps the ones the shell acts on.

use std::sync::{Arc, Mutex};

use alacritty_terminal::event::{Event, EventListener};

pub(crate) type EventQueue = Arc<Mutex<Vec<Event>>>;

#[derive(Debug, Clone)]
pub(crate) struct EventSink {
    queue: EventQueue,
}

impl EventSink {
    pub(crate) fn new(queue: EventQueue) -> Self {
        Self { queue }
    }
}

impl EventListener for EventSink {
    fn send_event(
        &self,
        event: Event,
    ) {
        let kept = matches!(
            event,
            Event::Title(_) | Event::ResetTitle | Event::Bell | Event::ClipboardStore(..)
        );
        if !kept {
            return;
        }
        if let Ok(mut queue) = self.queue.lock() {
            queue.push(event);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn replies_are_dropped_and_titles_kept() {
        let queue = EventQueue::default();
        let sink = EventSink::new(EventQueue::clone(&queue));
        sink.send_event(Event::PtyWrite("\x1b[?1;2c".into()));
        sink.send_event(Event::Title("vim".into()));
        let kept = queue.lock().map_or(0, |queue| queue.len());
        assert_eq!(kept, 1, "only the title survives");
    }
}
