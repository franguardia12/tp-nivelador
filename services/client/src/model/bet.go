package model

// Bet contains the agency-independent fields of one lottery bet.
// The agency is registered once for the whole client connection.
type Bet struct {
	FirstName string
	LastName  string
	Document  uint64
	Birthdate string
	Number    uint32
}
