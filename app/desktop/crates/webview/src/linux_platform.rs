use std::sync::mpsc;
use std::thread;

use gtk::glib;

use crate::bounds::Bounds;
use crate::error::{Error, Result};
use crate::gtk_state::GtkState;
use crate::host_config::HostConfig;
use crate::parent_window::ParentWindow;
use crate::platform::Platform;
use crate::visibility::Visibility;
use crate::web_view_config::WebViewConfig;
use crate::web_view_id::WebViewId;

type Job = Box<dyn FnOnce(&mut GtkState) + Send>;

/// Linux backend. WebKitGTK may only be touched from the thread that initialised GTK, so
/// every web view lives on a dedicated GTK thread and calls are jobs posted to it.
#[derive(Debug)]
pub(crate) struct LinuxPlatform {
    jobs: async_channel::Sender<Job>,
}

impl LinuxPlatform {
    pub(crate) fn start(config: HostConfig) -> Result<Self> {
        let (jobs, inbox) = async_channel::unbounded::<Job>();
        let (ready_tx, ready_rx) = mpsc::channel();
        thread::Builder::new()
            .name("crowbar-gtk".into())
            .spawn(move || run_gtk_thread(config, &inbox, &ready_tx))
            .map_err(|error| Error::PlatformThread(error.to_string()))?;
        ready_rx.recv().map_err(|_| Error::HostStopped)??;
        Ok(Self { jobs })
    }

    fn post(&self, job: impl FnOnce(&mut GtkState) + Send + 'static) -> Result<()> {
        self.jobs
            .send_blocking(Box::new(job))
            .map_err(|_| Error::HostStopped)
    }

    fn post_logged(
        &self,
        what: &'static str,
        job: impl FnOnce(&mut GtkState) -> Result<()> + Send + 'static,
    ) -> Result<()> {
        self.post(move |state| log_failure(what, job(state)))
    }
}

fn log_failure(what: &'static str, result: Result<()>) {
    if let Err(error) = result {
        tracing::warn!(%error, what, "web view operation failed");
    }
}

fn answer_create(reply: &mpsc::Sender<Result<()>>, result: Result<()>) {
    if reply.send(result).is_err() {
        tracing::warn!("web view created for a caller that stopped waiting");
    }
}

fn run_gtk_thread(
    config: HostConfig,
    inbox: &async_channel::Receiver<Job>,
    ready: &mpsc::Sender<Result<()>>,
) {
    if let Err(error) = gtk::init() {
        if ready
            .send(Err(Error::PlatformThread(error.to_string())))
            .is_err()
        {
            tracing::error!("gtk failed to initialise and nobody is waiting");
        }
        return;
    }
    let inbox = inbox.clone();
    let mut state = GtkState::new(config);
    glib::MainContext::default().spawn_local(async move {
        while let Ok(job) = inbox.recv().await {
            job(&mut state);
        }
        gtk::main_quit();
    });
    if ready.send(Ok(())).is_err() {
        return;
    }
    gtk::main();
}

impl Platform for LinuxPlatform {
    fn create(&self, id: WebViewId, parent: ParentWindow, config: WebViewConfig) -> Result<()> {
        let (reply, answer) = mpsc::channel();
        self.post(move |state| answer_create(&reply, state.create(id, parent, config)))?;
        answer.recv().map_err(|_| Error::HostStopped)?
    }

    fn set_bounds(&self, id: WebViewId, bounds: Bounds) -> Result<()> {
        self.post_logged("set_bounds", move |state| state.set_bounds(id, bounds))
    }

    fn set_visibility(&self, id: WebViewId, visibility: Visibility) -> Result<()> {
        self.post_logged("set_visibility", move |state| {
            state.set_visibility(id, visibility)
        })
    }

    fn eval(&self, id: WebViewId, script: &str) -> Result<()> {
        let script = script.to_owned();
        self.post_logged("eval", move |state| state.eval(id, &script))
    }

    fn navigate(&self, id: WebViewId, url: &str) -> Result<()> {
        let url = url.to_owned();
        self.post_logged("navigate", move |state| state.navigate(id, &url))
    }

    fn focus(&self, id: WebViewId) -> Result<()> {
        self.post_logged("focus", move |state| state.focus(id))
    }

    fn focus_parent(&self, id: WebViewId) -> Result<()> {
        self.post_logged("focus_parent", move |state| state.focus_parent(id))
    }

    fn destroy(&self, id: WebViewId) -> Result<()> {
        self.post_logged("destroy", move |state| state.destroy(id))
    }
}

#[cfg(test)]
#[path = "linux_platform_tests.rs"]
mod tests;
