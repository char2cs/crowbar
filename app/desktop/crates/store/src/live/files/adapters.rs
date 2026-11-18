//! The shell data layer's real machine: the disk.

mod disk_files;

pub(crate) use crate::live::files::adapters::disk_files::DiskFiles;
