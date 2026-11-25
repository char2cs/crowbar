//! In-memory demo data behind the Source and Sink ports, for tests and the gallery (feature fixtures).

#[cfg(feature = "fixtures")]
mod demo_shell;
#[cfg(feature = "fixtures")]
mod panel_reads;
#[cfg(feature = "fixtures")]
mod request_reads;
#[cfg(feature = "fixtures")]
mod rows;
#[cfg(feature = "fixtures")]
mod space_reads;

#[cfg(feature = "fixtures")]
pub use demo_shell::DemoShell;
