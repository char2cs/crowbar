//! The shell's side of the daemon port: `ShellTransport`, stream access and reply framing.

mod daemon_wire;
mod host_limits;
mod reply;
mod shell_transport;
mod stream_access;

pub use shell_transport::ShellTransport;
pub use stream_access::StreamAccess;
