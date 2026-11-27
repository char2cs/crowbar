use std::rc::Rc;

use gpui::{App, Window};

use crate::intent::Intent;

type Delivery = dyn Fn(Intent, &mut Window, &mut App);

#[derive(Clone)]
pub(crate) struct Sink(Rc<Delivery>);

impl Sink {
    pub(crate) fn new(delivery: impl Fn(Intent, &mut Window, &mut App) + 'static) -> Self {
        Self(Rc::new(delivery))
    }

    pub(crate) fn deliver(
        &self,
        intent: Intent,
        (window, cx): (&mut Window, &mut App),
    ) {
        (self.0)(intent, window, cx);
    }

    pub(crate) fn send<E>(
        &self,
        intent: Intent,
    ) -> impl Fn(&E, &mut Window, &mut App) + 'static {
        let sink = self.clone();
        move |_, window, cx| sink.deliver(intent.clone(), (window, cx))
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::intent::runner::Runner as _;
    use crate::test_support::{inspect, open};

    fn selected(
        harness: &crate::test_support::Harness,
        cx: &mut gpui::VisualTestContext,
    ) -> Option<String> {
        inspect(harness, cx, |modal| {
            modal.state.page().map(|page| page.id().to_string())
        })
    }

    #[gpui::test]
    fn deliver_and_send_both_reach_the_modal(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let owner = harness.modal.clone();
        let sink = Sink::new(move |intent, window, cx| {
            owner.update(cx, |modal, cx| modal.run(intent, window, cx));
        });
        cx.update(|window, cx| sink.deliver(Intent::Select("git".into()), (window, cx)));
        assert_eq!(selected(&harness, cx), Some("git".to_owned()));
        let handler = sink.send::<()>(Intent::Select("editor".into()));
        cx.update(|window, cx| handler(&(), window, cx));
        assert_eq!(selected(&harness, cx), Some("editor".to_owned()));
    }
}
