package gateway

// BindAddrs exposes bindAddrs, so the bind plan for a network-exposed host is
// testable without binding a non-loopback address on the test machine.
var BindAddrs = bindAddrs
