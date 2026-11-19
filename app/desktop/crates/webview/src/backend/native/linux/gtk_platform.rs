use std::sync::mpsc;
use std::thread;

use gtk::glib;
use platform::InputFocus;

use crate::backend::host_config::HostConfig;
use crate::backend::native::linux::gtk_state::GtkState;
use crate::backend::platform::Platform;
use crate::error::{Error, Result};

type Job = Box<dyn FnOnce(&mut GtkState) + Send>;
type Ready = mpsc::Sender<Result<()>>;

#[derive(Debug)]
pub(crate) struct GtkPlatform {
    pub(crate) jobs: async_channel::Sender<Job>,
    pub(crate) stopped: Option<mpsc::Receiver<()>>,
}

impl GtkPlatform {
    pub(crate) fn start(config: HostConfig) -> Result<Self> {
        let (jobs, inbox) = async_channel::unbounded::<Job>();
        let (ready_tx, ready_rx) = mpsc::channel();
        let (stopped_tx, stopped) = mpsc::channel();
        thread::Builder::new()
            .name("crowbar-gtk".into())
            .spawn(move || run_gtk_thread(config, (&inbox, &ready_tx), &stopped_tx))
            .map_err(|error| Error::PlatformThread(error.to_string()))?;
        ready_rx.recv().map_err(|error| {
            tracing::debug!(%error, "the platform thread stopped before it was ready");
            Error::HostStopped
        })??;
        Ok(Self {
            jobs,
            stopped: Some(stopped),
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

impl Drop for GtkPlatform {
    fn drop(&mut self) {
        self.jobs.close();
        let Some(stopped) = self.stopped.take() else {
            return;
        };
        if stopped.recv().is_err() {
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
    error: &dyn std::fmt::Display,
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
    (inbox, ready): (&async_channel::Receiver<Job>, &Ready),
    stopped: &mpsc::Sender<()>,
) {
    if !serve(config, inbox, ready) {
        return;
    }
    stopped.send(()).unwrap_or_default();
    outlive_webkit();
}

fn serve(
    config: HostConfig,
    inbox: &async_channel::Receiver<Job>,
    ready: &mpsc::Sender<Result<()>>,
) -> bool {
    if let Err(error) = gtk::init() {
        report_init_failure(ready, &error);
        return false;
    }
    let keyboard = match InputFocus::connect() {
        Ok(keyboard) => keyboard,
        Err(error) => {
            report_init_failure(ready, &error);
            return false;
        },
    };
    let inbox = inbox.clone();
    let mut state = GtkState::new(config, keyboard);
    glib::MainContext::default().spawn_local(async move {
        run_jobs(&inbox, &mut state).await;
        gtk::main_quit();
    });
    if ready.send(Ok(())).is_err() {
        return false;
    }
    gtk::main();
    true
}

// PLATFORM: linux WebKit's RunLoop exit hook deadlocks with threads still dispatching to it.
fn outlive_webkit() -> ! {
    loop {
        thread::park();
    }
}

impl Platform for GtkPlatform {}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::host_fixtures::silent_host_config;

    const DROP_WAIT: std::time::Duration = std::time::Duration::from_secs(5);

    #[test]
    fn dropping_the_host_does_not_wait_for_the_gtk_thread_to_end() {
        let (jobs, inbox) = async_channel::unbounded::<Job>();
        let (release, exit_hook) = mpsc::channel::<()>();
        let (stopped_tx, stopped) = mpsc::channel();
        thread::spawn(move || {
            drop(inbox.recv_blocking());
            stopped_tx.send(()).unwrap_or_default();
            exit_hook.recv().unwrap_or_default();
        });
        let platform = GtkPlatform {
            jobs,
            stopped: Some(stopped),
        };
        let (dropped_tx, dropped) = mpsc::channel();
        thread::spawn(move || {
            drop(platform);
            dropped_tx.send(()).unwrap_or_default();
        });
        let outcome = dropped.recv_timeout(DROP_WAIT);
        release.send(()).unwrap_or_default();
        assert!(
            outcome.is_ok(),
            "the drop waited on the thread's exit hooks"
        );
    }

    #[test]
    fn starting_without_a_display_is_a_typed_error() {
        if std::env::var_os("DISPLAY").is_some() {
            return;
        }
        assert!(
            matches!(
                GtkPlatform::start(silent_host_config()),
                Err(Error::PlatformThread(_))
            ),
            "no display"
        );
    }
}
