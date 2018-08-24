package bencode

import (
	"errors"
)

// RawMessage is a raw encoded bencode value.
// It implements Marshaler and Unmarshaler and can
// be used to delay bencode decoding or precompute a bencoding.
type RawMessage []byte

// MarshalBencode returns m as the bencoding of m.
func (m RawMessage) MarshalBencode() ([]byte, error) {
	if m == nil {
		return []byte(""), nil
	}
	return m, nil
}

// UnmarshalBencode sets *m to a copy of data.
func (m *RawMessage) UnmarshalBencode(data []byte) error {
	if m == nil {
		return errors.New("bencode.RawMessage: UnmarshalBencode on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}
