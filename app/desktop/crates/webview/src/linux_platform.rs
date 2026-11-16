use std::sync::mpsc;
use std::thread;
use std::time::{Duration, Instant};

use gtk::glib;

use crate::bounded_wait::{collect_within, wait_for};
use crate::bounds::Bounds;
use crate::create_reply::CreateReply;
use crate::error::{Error, Result};
use crate::gtk_state::GtkState;
use crate::host_config::HostConfig;
use crate::parent_window::ParentWindow;
use crate::platform::Platform;
use crate::snapshot::Snapshot;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;

const SNAPSHOT_PATIENCE: Duration = Duration::from_millis(400);
const CREATE_PATIENCE: Duration = Duration::from_secs(5);

type Job = Box<dyn FnOnce(&mut GtkState) + Send>;

#[derive(Debug)]
pub(crate) struct LinuxPlatform {
    jobs: async_channel::Sender<Job>,
    thread: Option<thread::JoinHandle<()>>,
}

impl LinuxPlatform {
    pub(crate) fn start(config: HostConfig) -> Result<Self> {
        let (jobs, inbox) = async_channel::unbounded::<Job>();
        let (ready_tx, ready_rx) = mpsc::channel();
        let thread = thread::Builder::new()
            .name("crowbar-gtk".into())
            .spawn(move || run_gtk_thread(config, &inbox, &ready_tx))
            .map_err(|error| Error::PlatformThread(error.to_string()))?;
        ready_rx.recv().map_err(|_| Error::HostStopped)??;
        Ok(Self {
            jobs,
            thread: Some(thread),
        })
    }

    fn post(
        &self,
        job: impl FnOnce(&mut GtkState) + Send + 'static,
    ) -> Result<()> {
        self.jobs
            .send_blocking(Box::new(job))
            .map_err(|_| Error::HostStopped)
    }

    fn ask_snapshot(
        &self,
        id: WebViewId,
    ) -> (Result<()>, SnapshotAnswer) {
        let (reply, answer) = mpsc::channel();
        (self.post(move |state| state.snapshot(id, reply)), answer)
    }

    fn post_logged(
        &self,
        what: &'static str,
        job: impl FnOnce(&mut GtkState) -> Result<()> + Send + 'static,
    ) -> Result<()> {
        self.post(move |state| log_failure(what, job(state)))
    }
}

impl Drop for LinuxPlatform {
    fn drop(&mut self) {
        self.jobs.close();
        let Some(thread) = self.thread.take() else {
            return;
        };
        if thread.join().is_err() {
            tracing::error!("the web view thread panicked while stopping");
        }
    }
}

fn log_failure(
    what: &'static str,
    result: Result<()>,
) {
    if let Err(error) = result {
        tracing::warn!(%error, what, "web view operation failed");
    }
}

type SnapshotAnswer = mpsc::Receiver<Result<Option<Snapshot>>>;

fn settle_snapshot(
    posted: Result<()>,
    reply: Option<Result<Option<Snapshot>>>,
) -> Result<Option<Snapshot>> {
    posted?;
    reply.unwrap_or_else(|| {
        tracing::debug!("a web view did not answer a snapshot in time");
        Ok(None)
    })
}

fn settle_create(
    state: &mut GtkState,
    reply: CreateReply,
    created: Result<()>,
) {
    let Some(abandoned) = reply.send(created) else {
        return;
    };
    log_failure("destroy", state.destroy(abandoned));
}

fn report_init_failure(
    ready: &mpsc::Sender<Result<()>>,
    error: &glib::BoolError,
) {
    let failure = Error::PlatformThread(error.to_string());
    if ready.send(Err(failure)).is_err() {
        tracing::error!("gtk failed to initialise and nobody is waiting");
    }
}

async fn run_jobs(
    inbox: &async_channel::Receiver<Job>,
    state: &mut GtkState,
) {
    while let Ok(job) = inbox.recv().await {
        job(state);
    }
}

fn run_gtk_thread(
    config: HostConfig,
    inbox: &async_channel::Receiver<Job>,
    ready: &mpsc::Sender<Result<()>>,
) {
    if let Err(error) = gtk::init() {
        report_init_failure(ready, &error);
        return;
    }
    let inbox = inbox.clone();
    let mut state = GtkState::new(config);
    glib::MainContext::default().spawn_local(async move {
        run_jobs(&inbox, &mut state).await;
        gtk::main_quit();
    });
    if ready.send(Ok(())).is_err() {
        return;
    }
    gtk::main();
}

impl Platform for LinuxPlatform {
    fn create(
        &self,
        id: WebViewId,
        parent: ParentWindow,
        config: WebViewConfig,
    ) -> Result<()> {
        let (reply, answer) = mpsc::channel();
        self.post(move |state| {
            let created = state.create(id, parent, &config);
            settle_create(state, CreateReply { reply, id }, created);
        })?;
        wait_for(&answer, CREATE_PATIENCE)
    }

    fn set_bounds(
        &self,
        id: WebViewId,
        bounds: Bounds,
    ) -> Result<()> {
        self.post_logged("set_bounds", move |state| state.set_bounds(id, bounds))
    }

    fn set_visibility(
        &self,
        id: WebViewId,
        visibility: Visibility,
    ) -> Result<()> {
        self.post_logged("set_visibility", move |state| {
            state.set_visibility(id, visibility)
        })
    }

    fn eval(
        &self,
        id: WebViewId,
        script: &str,
    ) -> Result<()> {
        let script = script.to_owned();
        self.post_logged("eval", move |state| state.eval(id, &script))
    }

    fn navigate(
        &self,
        id: WebViewId,
        url: &str,
    ) -> Result<()> {
        let url = url.to_owned();
        self.post_logged("navigate", move |state| state.navigate(id, &url))
    }

    fn focus(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.post_logged("focus", move |state| state.focus(id))
    }

    fn focus_parent(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.post_logged("focus_parent", move |state| state.focus_parent(id))
    }

    fn snapshot(
        &self,
        id: WebViewId,
    ) -> Result<Option<Snapshot>> {
        self.snapshot_all(&[id]).pop().unwrap_or(Ok(None))
    }

    fn snapshot_all(
        &self,
        ids: &[WebViewId],
    ) -> Vec<Result<Option<Snapshot>>> {
        let (posted, answers): (Vec<_>, Vec<_>) =
            ids.iter().map(|id| self.ask_snapshot(*id)).unzip();
        let replies = collect_within(&answers, SNAPSHOT_PATIENCE, Instant::now);
        posted
            .into_iter()
            .zip(replies)
            .map(|(posted, reply)| settle_snapshot(posted, reply))
            .collect()
    }

    fn clear_browsing_data(&self) -> Result<()> {
        self.post_logged("clear_browsing_data", GtkState::clear_browsing_data)
    }

    fn destroy(
        &self,
        id: WebViewId,
    ) -> Result<()> {
        self.post_logged("destroy", move |state| state.destroy(id))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::protocol_handler::{ProtocolHandler, ProtocolResponder};
    use crate::protocol_request::ProtocolRequest;

    struct Silent;

    impl ProtocolHandler for Silent {
        fn handle(
            &self,
            _request: ProtocolRequest,
            _respond: ProtocolResponder,
        ) {
        }
    }

    #[test]
    fn starting_without_a_display_is_a_typed_error() {
        if std::env::var_os("DISPLAY").is_some() {
            return;
        }
        let config = HostConfig {
            scheme: "crowbar".into(),
            protocol: std::sync::Arc::new(Silent),
            data_dir: None,
        };
        assert!(
            matches!(LinuxPlatform::start(config), Err(Error::PlatformThread(_))),
            "no display"
        );
    }
}
