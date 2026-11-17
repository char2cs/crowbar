use serde_json::Value;
use tokio::sync::mpsc;

pub(crate) struct Daemon {
    to_window: mpsc::UnboundedSender<String>,
    from_window: mpsc::UnboundedReceiver<String>,
}

impl Daemon {
    pub(crate) fn new(
        to_window: mpsc::UnboundedSender<String>,
        from_window: mpsc::UnboundedReceiver<String>,
    ) -> Self {
        Self {
            to_window,
            from_window,
        }
    }

    pub(crate) async fn next(&mut self) -> Value {
        let text = self
            .from_window
            .recv()
            .await
            .expect("the window sent a frame");
        serde_json::from_str(&text).expect("the window sent json")
    }

    pub(crate) fn nothing_waits(&mut self) -> bool {
        self.from_window.try_recv().is_err()
    }

    pub(crate) fn say(
        &self,
        frame: &Value,
    ) {
        self.to_window
            .send(frame.to_string())
            .expect("the window listens");
    }
}
