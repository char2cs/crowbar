//! The Devices page: whether other devices may connect to this computer's daemon, and the ones
//! paired with it. The daemon holds both; nothing here is written to the settings file.

mod access;
pub(crate) mod device;
mod paired;
