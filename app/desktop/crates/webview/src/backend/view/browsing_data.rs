use crate::error::Result;

pub trait BrowsingData {
    fn clear_browsing_data(&self) -> Result<()>;
}
