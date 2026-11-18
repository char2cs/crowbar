//! The shell data layer's real machine: the disk.

mod disk_files;

pub(crate) use crate::live::file_store::adapters::disk_files::DiskFiles;
