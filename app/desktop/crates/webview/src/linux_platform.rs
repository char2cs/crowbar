use std::sync::mpsc;
use std::thread;

use gtk::glib;

use crate::error::{Error, Result};
use crate::gtk_state::GtkState;
use crate::host_config::HostConfig;
use crate::platform::Platform;

type Job = Box<dyn FnOnce(&mut GtkState) + Send>;

#[derive(Debug)]
pub(crate) struct LinuxPlatform {
    pub(crate) jobs: async_channel::Sender<Job>,
    pub(crate) thread: Option<thread::JoinHandle<()>>,
}

impl LinuxPlatform {
    pub(crate) fn start(config: HostConfig) -> Result<Self> {
        let (jobs, inbox) = async_channel::unbounded::<Job>();
        let (ready_tx, ready_rx) = mpsc::channel();
        let thread = thread::Builder::new()
            .name("crowbar-gtk".into())
            .spawn(move || run_gtk_thread(config, &inbox, &ready_tx))
            .map_err(|error| Error::PlatformThread(error.to_string()))?;
        ready_rx.recv().map_err(|error| {
            tracing::debug!(%error, "the platform thread stopped before it was ready");
            Error::HostStopped
        })??;
        Ok(Self {
            jobs,
            thread: Some(thread),
        })
    }

    pub(crate) fn post(
        &self,
        job: impl FnOnce(&mut GtkState) + Send + 'static,
    ) -> Result<()> {
        self.jobs.send_blocking(Box::new(job)).map_err(|error| {
            tracing::debug!(%error, "the platform thread has stopped");
            Error::HostStopped
        })
    }

    pub(crate) fn post_logged(
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

pub(crate) fn log_failure(
    what: &'static str,
    result: Result<()>,
) {
    if let Err(error) = result {
        tracing::warn!(%error, what, "web view operation failed");
    }
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

impl Platform for LinuxPlatform {}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::host_fixtures::silent_host_config;

    #[test]
    fn starting_without_a_display_is_a_typed_error() {
        if std::env::var_os("DISPLAY").is_some() {
            return;
        }
        assert!(
            matches!(
                LinuxPlatform::start(silent_host_config()),
                Err(Error::PlatformThread(_))
            ),
            "no display"
        );
    }
}
