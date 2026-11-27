#[cfg(target_os = "linux")]
mod bitmap_rows;
#[cfg(target_os = "linux")]
mod row_layout;
#[cfg(target_os = "linux")]
mod surface_pixels;

#[cfg(target_os = "linux")]
pub(crate) use surface_pixels::snapshot_of;
