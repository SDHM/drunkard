package lb

// Address represents a server the client connects to.
// This is the EXPERIMENTAL API and may be changed or extended in the future.
type Address struct {
	// Addr is the server address on which a connection will be established.
	Addr string
	// Metadata is the information associated with Addr, which may be used
	// to make load balancing decision.
	Metadata interface{}
}
