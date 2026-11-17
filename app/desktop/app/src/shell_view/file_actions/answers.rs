use gpui::{AsyncWindowContext, WeakEntity};

use super::handlers::Handlers;
use crate::file_reply::Reply;
use crate::logged::closed_view;

pub(super) async fn deliver<V: 'static, T: Send + 'static>(
    reply: Reply<T>,
    handlers: Handlers<V, T>,
    this: WeakEntity<V>,
    cx: &mut AsyncWindowContext,
) {
    let Ok(answer) = reply.recv().await else {
        return;
    };
    let Handlers { then, failed } = handlers;
    let delivered = this.update_in(cx, |this, window, cx| match answer {
        Ok(value) => then(this, value, window, cx),
        Err(reason) => failed(this, reason.to_string(), cx),
    });
    closed_view(&delivered, "the answer");
}

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, Render, TestAppContext};

    use gpui::{Context, Window};

    use super::super::handlers::Then;
    use super::*;
    use crate::file_reply;

    #[derive(Default)]
    struct Probe {
        heard: Vec<String>,
    }

    fn fail(
        probe: &mut Probe,
        reason: String,
        _: &mut Context<'_, Probe>,
    ) {
        probe.heard.push(reason);
    }

    impl Render for Probe {
        fn render(
            &mut self,
            _: &mut Window,
            _: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            gpui::div()
        }
    }

    fn start(
        reply: Reply<u8>,
        then: Then<Probe, u8>,
        (window, cx): (&mut Window, &mut Context<'_, Probe>),
    ) {
        cx.spawn_in(window, async move |this, cx| {
            deliver(reply, Handlers { then, failed: fail }, this, cx).await;
        })
        .detach();
    }

    fn heard_after(
        cx: &mut TestAppContext,
        answer: Result<u8, shell_data::FileError>,
    ) -> Vec<String> {
        let (probe, cx) = cx.add_window_view(|_, _| Probe::default());
        let (done, reply) = file_reply::channel();
        done(answer);
        let then: Then<Probe, u8> = Box::new(|probe, value, _, _| {
            probe.heard.push(format!("got {value}"));
        });
        probe.update_in(cx, |_, window, cx| start(reply, then, (window, cx)));
        cx.run_until_parked();
        probe.read_with(cx, |probe, _| probe.heard.clone())
    }

    #[gpui::test]
    fn an_answer_reaches_the_view_and_a_failure_reaches_its_handler(cx: &mut TestAppContext) {
        assert_eq!(heard_after(cx, Ok(7)), ["got 7"]);
        let refused = shell_data::FileError::Local {
            path: "disk".into(),
            source: std::io::Error::other("no space"),
        };
        assert_eq!(heard_after(cx, Err(refused)), ["disk: no space"]);
    }
}
