package bencode

// Marshal returns the bencode of v.
func Marshal(v interface{}) ([]byte, error) {
	return nil, nil
}

// Marshaler is the interface implemented by types
// that can marshal themselves into valid bencode.
type Marshaler interface {
	MarshalBencode() ([]byte, error)
}
