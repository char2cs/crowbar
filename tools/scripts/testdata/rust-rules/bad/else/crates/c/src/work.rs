/// Works.
pub fn work(a: u32) -> u32 {
    if a == 0 {
        return 1;
    } else {
        return a;
    }
}
#[cfg(test)]
mod tests {}
