//! Live relay connections and their cancellation.

pub(crate) mod connection_book;
mod connection_guard;
mod connection_release;
mod live;
mod registry;

pub(crate) use connection_book::ConnectionBook;
pub(crate) use connection_guard::ConnectionGuard;
