package constant

// BackendCarrier is a mutable holder for the selected backend address.
// It is set once in the Director and written by RoundTrip on each attempt,
// allowing the outer middleware to read the final backend after the response.
type BackendCarrier struct {
	Addr string
}
