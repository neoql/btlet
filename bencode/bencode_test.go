package bencode

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"time"
)

type myBoolPtrType bool

// MarshalBencode implements Marshaler.MarshalBencode
func (mbt *myBoolPtrType) MarshalBencode() ([]byte, error) {
	var c string
	if *mbt {
		c = "y"
	} else {
		c = "n"
	}

	return Marshal(c)
}

// UnmarshalBencode implements Unmarshaler.UnmarshalBencode
func (mbt *myBoolPtrType) UnmarshalBencode(b []byte) error {
	var str string
	err := Unmarshal(b, &str)
	if err != nil {
		return err
	}

	switch str {
	case "y":
		*mbt = true
	case "n":
		*mbt = false
	default:
		err = errors.New("invalid myBoolType")
	}

	return err
}

type myStringType string

// UnmarshalBencode implements Unmarshaler.UnmarshalBencode
func (mst *myStringType) UnmarshalBencode(b []byte) error {
	var raw []byte
	err := Unmarshal(b, &raw)
	if err != nil {
		return err
	}

	*mst = myStringType(raw)
	return nil
}

type mySliceType []interface{}

// UnmarshalBencode implements Unmarshaler.UnmarshalBencode
func (mst *mySliceType) UnmarshalBencode(b []byte) error {
	m := make(map[string]interface{})
	err := Unmarshal(b, &m)
	if err != nil {
		return err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	raw := make([]interface{}, 0, len(m)*2)
	for _, key := range keys {
		raw = append(raw, key, m[key])
	}

	*mst = mySliceType(raw)
	return nil
}

type myTextStringType string

// UnmarshalText implements TextUnmarshaler.UnmarshalText
func (mst *myTextStringType) UnmarshalText(b []byte) error {
	*mst = myTextStringType(bytes.TrimPrefix(b, []byte("foo_")))
	return nil
}

type myTextSliceType []string

// UnmarshalText implements TextUnmarshaler.UnmarshalText
func (mst *myTextSliceType) UnmarshalText(b []byte) error {
	raw := string(b)
	*mst = strings.Split(raw, ",")
	return nil
}

type myBoolType bool

// MarshalBencode implements Marshaler.MarshalBencode
func (mbt myBoolType) MarshalBencode() ([]byte, error) {
	var c string
	if mbt {
		c = "y"
	} else {
		c = "n"
	}

	return Marshal(c)
}

// UnmarshalBencode implements Unmarshaler.UnmarshalBencode
func (mbt *myBoolType) UnmarshalBencode(b []byte) error {
	var str string
	err := Unmarshal(b, &str)
	if err != nil {
		return err
	}

	switch str {
	case "y":
		*mbt = true
	case "n":
		*mbt = false
	default:
		err = errors.New("invalid myBoolType")
	}

	return err
}

type myBoolTextType bool

// MarshalText implements TextMarshaler.MarshalText
func (mbt myBoolTextType) MarshalText() ([]byte, error) {
	if mbt {
		return []byte("y"), nil
	}

	return []byte("n"), nil
}

// UnmarshalText implements TextUnmarshaler.UnmarshalText
func (mbt *myBoolTextType) UnmarshalText(b []byte) error {
	switch string(b) {
	case "y":
		*mbt = true
	case "n":
		*mbt = false
	default:
		return errors.New("invalid myBoolType")
	}
	return nil
}

type myTimeType struct {
	time.Time
}

// MarshalBencode implements Marshaler.MarshalBencode
func (mtt myTimeType) MarshalBencode() ([]byte, error) {
	return Marshal(mtt.Time.Unix())
}

// UnmarshalBencode implements Unmarshaler.UnmarshalBencode
func (mtt *myTimeType) UnmarshalBencode(b []byte) error {
	var epoch int64
	err := Unmarshal(b, &epoch)
	if err != nil {
		return err
	}

	mtt.Time = time.Unix(epoch, 0)
	return nil
}

type errorMarshalType struct{}

// MarshalBencode implements Marshaler.MarshalBencode
func (emt errorMarshalType) MarshalBencode() ([]byte, error) {
	return nil, errors.New("oops")
}

// UnmarshalBencode implements Unmarshaler.UnmarshalBencode
func (emt errorMarshalType) UnmarshalBencode([]byte) error {
	return errors.New("oops")
}

type errorTextMarshalType struct{}

// MarshalText implements TextMarshaler.MarshalText
func (emt errorTextMarshalType) MarshalText() ([]byte, error) {
	return nil, errors.New("oops")
}

// UnmarshalText implements TextUnmarshaler.UnmarshalText
func (emt errorTextMarshalType) UnmarshalText([]byte) error {
	return errors.New("oops")
}
