use editorview::DiskTake;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Adoption {
    Replaces(DiskTake),
    Stays(DiskTake),
}

impl Adoption {
    pub(crate) const fn took(self) -> DiskTake {
        match self {
            Self::Replaces(took) | Self::Stays(took) => took,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn either_way_it_tells_what_the_file_pane_took() {
        for took in [DiskTake::Kept, DiskTake::Taken, DiskTake::Conflicted] {
            assert_eq!(Adoption::Replaces(took).took(), took);
            assert_eq!(Adoption::Stays(took).took(), took);
        }
    }
}
