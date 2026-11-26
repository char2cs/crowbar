use crate::backend::native::linux::gtk_platform::GtkPlatform;
use crate::backend::native::linux::gtk_platform::channels::Job;

pub(crate) fn idle_platform() -> (GtkPlatform, async_channel::Receiver<Job>) {
    let (jobs, inbox) = async_channel::unbounded();
    let stopped = None;
    (GtkPlatform { jobs, stopped }, inbox)
}
