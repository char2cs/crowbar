//! The on/off phase of a field's caret and the one timer that flips it at its next edge.

use gpui::{Context, Task};

use crate::control::editor::field::Blinking;
use crate::foundation::Pose;
use crate::foundation::tokens::timing;

#[derive(Default)]
pub(crate) struct Blink {
    hidden: bool,
    edge: Option<Task<()>>,
}

impl Blink {
    pub(crate) fn is_shown(&self) -> bool {
        !self.hidden
    }

    pub(crate) fn restart(&mut self) {
        self.hidden = false;
        self.edge = None;
    }

    fn flip(&mut self) {
        self.hidden = !self.hidden;
        self.edge = None;
    }
}

pub(crate) fn tend<T: Blinking>(
    field: &mut T,
    lit: Pose,
    cx: &mut Context<'_, T>,
) {
    if lit == Pose::Off {
        field.blink_mut().restart();
        return;
    }
    if field.blink().edge.is_some() {
        return;
    }
    let edge = cx.spawn(async move |field, cx| {
        cx.background_executor().timer(timing::CARET_BLINK).await;
        field.update(cx, pass_edge).unwrap_or_default();
    });
    field.blink_mut().edge = Some(edge);
}

fn pass_edge<T: Blinking>(
    field: &mut T,
    cx: &mut Context<'_, T>,
) {
    field.blink_mut().flip();
    cx.notify();
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_restart_shows_the_caret_and_cancels_the_pending_edge() {
        let mut blink = Blink::default();
        blink.flip();
        assert!(!blink.is_shown(), "a flip hides a shown caret");
        blink.restart();
        assert!(blink.is_shown(), "a restart shows it again");
        assert!(blink.edge.is_none(), "and drops the timer");
    }
}
