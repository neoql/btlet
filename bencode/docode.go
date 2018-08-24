package bencode

import (
	"bufio"
	"bytes"
	"encoding"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

var (
	reflectByteSliceType = reflect.TypeOf([]byte(nil))
	reflectStringType    = reflect.TypeOf("")
)

// Unmarshal parses the bencode data and stores the result in the value pointed to by v.
// If v is nil or not a pointer, Unmarshal returns an InvalidUnmarshalError.
func Unmarshal(data []byte, v interface{}) error {
	dec := NewDecoder(bytes.NewReader(data))
	return dec.Decode(v)
}

// Unmarshaler is the interface implemented by types
// that can unmarshal a bencode description of themselves.
type Unmarshaler interface {
	UnmarshalBencode([]byte) error
}

// A Decoder reads and decodes bencode values from an input stream.
type Decoder struct {
	r                     *bufio.Reader
	offset                int
	disallowUnknownFields bool
	disallowUnorderedKeys bool
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// Decode reads the next bencoded value from its input and stores it in the value pointed to by v.
func (dec *Decoder) Decode(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return &InvalidUnmarshalError{reflect.TypeOf(v)}
	}
	// We decode rv not rv.Elem because the Unmarshaler interface
	// test must be applied at the top level of the value.
	return dec.decode(rv)
}

// DisallowUnknownFields causes the Decoder to return an
// error when the destination is a struct and the input contains
// object keys which do not match any non-ignored, exported fields in the destination.
func (dec *Decoder) DisallowUnknownFields() {
	dec.disallowUnknownFields = true
}

// DisallowUnorderedKeys will cause the decoder to fail when encountering
// unordered keys. The default is to not fail.
func (dec *Decoder) DisallowUnorderedKeys() {
	dec.disallowUnorderedKeys = true
}

// BytesParsed returns the number of bytes that have actually been parsed
func (dec *Decoder) BytesParsed() int {
	return dec.offset
}

func (dec *Decoder) decode(val reflect.Value) error {
	v := val

	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		v = v.Addr()
	}
	for {
		if v.Kind() == reflect.Interface && !v.IsNil() {
			t := v.Elem()
			if t.Kind() == reflect.Ptr && !t.IsNil() {
				v = t
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}

		iface := v.Interface()
		switch e := iface.(type) {
		case Unmarshaler:
			buf := bytes.Buffer{}
			err := dec.readValueInto(&buf)
			if err != nil {
				return err
			}
			return e.UnmarshalBencode(buf.Bytes())
		case encoding.TextUnmarshaler:
			var buf []byte
			err := dec.decodeString(reflect.ValueOf(&buf).Elem())
			if err != nil {
				return err
			}
			return e.UnmarshalText(buf)
		}

		v = v.Elem()
	}

	next, err := dec.peekByte()
	if err != nil {
		return err
	}

	switch next {
	case 'i':
		return dec.decodeInt(v)
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return dec.decodeString(v)
	case 'l':
		return dec.decodeList(v)
	case 'd':
		return dec.decodeDict(v)
	default:
		return &SyntaxError{"Invalid input", int64(dec.offset)}
	}
}

func (dec *Decoder) decodeInt(val reflect.Value) error {
	ch, err := dec.readByte()
	if err != nil {
		return err
	}
	if ch != 'i' {
		panic("got not an i when peek returned an i")
	}

	line, err := dec.readBytes('e')
	if err != nil {
		return err
	}
	digits := string(line[:len(line)-1])

	switch val.Kind() {
	default:
		return &UnmarshalTypeError{
			Value:  "integer",
			Type:   val.Type(),
			Offset: int64(dec.offset),
		}
	case reflect.Interface:
		n, err := strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return err
		}
		val.Set(reflect.ValueOf(n))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(digits, 10, 64)
		if err != nil {
			return err
		}
		val.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(digits, 10, 64)
		if err != nil {
			return err
		}
		val.SetUint(n)
	case reflect.Bool:
		n, err := strconv.ParseUint(digits, 10, 64)
		if err != nil {
			return err
		}
		val.SetBool(n != 0)
	}

	return nil
}

func (dec *Decoder) decodeString(val reflect.Value) error {
	//read until a colon to get the number of digits to read after
	line, err := dec.readBytes(':')
	if err != nil {
		return err
	}

	//parse it into an int for making a slice
	l32, err := strconv.ParseInt(string(line[:len(line)-1]), 10, 32)
	l := int(l32)
	if err != nil {
		return err
	}
	if l < 0 {
		return &SyntaxError{fmt.Sprintf("invalid negative string length: %d", l), int64(dec.offset)}
	}

	//read exactly l bytes out and make our string
	buf := make([]byte, l)
	_, err = dec.readFull(buf)
	if err != nil {
		return err
	}

	switch val.Kind() {
	default:
		return &UnmarshalTypeError{
			Value:  "string",
			Offset: int64(dec.offset),
			Type:   val.Type(),
		}
	case reflect.Slice:
		if val.Type() != reflectByteSliceType {
			return &UnmarshalTypeError{
				Value:  "string",
				Offset: int64(dec.offset),
				Type:   val.Type(),
			}
		}
		val.SetBytes(buf)
	case reflect.String:
		val.SetString(string(buf))
	case reflect.Interface:
		val.Set(reflect.ValueOf(string(buf)))
	}
	return nil
}

func (dec *Decoder) decodeList(val reflect.Value) error {
	if val.Kind() == reflect.Interface {
		var x []interface{}
		defer func(p reflect.Value) { p.Set(val) }(val)
		val = reflect.ValueOf(&x).Elem()
	}

	if val.Kind() != reflect.Array && val.Kind() != reflect.Slice {
		return &UnmarshalTypeError{
			Value:  "list",
			Offset: int64(dec.offset),
			Type:   val.Type(),
		}
	}

	//read out the l that prefixes the list
	ch, err := dec.readByte()
	if err != nil {
		return err
	}
	if ch != 'l' {
		panic("got something other than a list head after a peek")
	}

	for i := 0; ; i++ {
		next, err := dec.peekByte()
		if err != nil {
			return err
		}
		if next == 'e' {
			_, err := dec.readByte()
			return err
		}

		switch val.Kind() {
		case reflect.Array:
			if i < val.Cap() {
				err := dec.decode(val.Index(i))
				if err != nil {
					return err
				}
				continue
			}
			err := dec.readValueInto(nil)
			if err != nil {
				return err
			}
		case reflect.Slice:
			if i == 0 {
				val.SetLen(0)
			}
			v := reflect.New(val.Type().Elem())
			err := dec.decode(v.Elem())
			if err != nil {
				return err
			}
			val.Set(reflect.Append(val, v.Elem()))
		}
	}
}

func (dec *Decoder) decodeDict(val reflect.Value) error {
	if val.Kind() == reflect.Interface {
		var x map[string]interface{}
		defer func(p reflect.Value) { p.Set(val) }(val)
		val = reflect.ValueOf(&x).Elem()
	}

	//consume the head token
	ch, err := dec.readByte()
	if err != nil {
		return err
	}
	if ch != 'd' {
		panic("got an incorrect token when it was checked already")
	}

	t := val.Type()

	var isMap bool
	var m map[string]reflect.Value
	switch val.Kind() {
	default:
		return &UnmarshalTypeError{
			Value:  "dict",
			Offset: int64(dec.offset),
			Type:   val.Type(),
		}
	case reflect.Map:
		isMap = true
		if t.Key() != reflectStringType {
			return &UnmarshalTypeError{
				Value:  "dict",
				Offset: int64(dec.offset),
				Type:   val.Type(),
			}
		}

		if val.IsNil() {
			val.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:
		m = make(map[string]reflect.Value)
		wrapStruct2Map(val, m)
	}

	lastKey := ""
	for {
		next, err := dec.peekByte()
		if err != nil {
			return err
		}
		if next == 'e' {
			_, err := dec.readByte()
			return err
		}
		var key string
		err = dec.decodeString(reflect.ValueOf(&key).Elem())
		if err != nil {
			return err
		}

		if dec.disallowUnorderedKeys && lastKey > key {
			return &SyntaxError{
				fmt.Sprintf("unordered dictionary: %q appears before %q", lastKey, key), int64(dec.offset),
			}
		}
		lastKey = key

		var e reflect.Value
		if isMap {
			e = val.MapIndex(reflect.ValueOf(key))
			if !e.IsValid() {
				e = reflect.New(t.Elem()).Elem()
			}
		} else {
			var ok bool
			e, ok = m[key]
			if !ok {
				if dec.disallowUnknownFields {
					return &SyntaxError{"disallow unknow fields", int64(dec.offset)}
				}
				err := dec.readValueInto(nil)
				if err != nil {
					return err
				}
				continue
			}
		}

		err = dec.decode(e)
		if err != nil {
			return err
		}

		if isMap {
			val.SetMapIndex(reflect.ValueOf(key), e)
		}
	}
}

func wrapStruct2Map(val reflect.Value, m map[string]reflect.Value) {
	t := val.Type()
	for i := 0; i < val.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		v := val.FieldByIndex(f.Index)
		if f.Anonymous && f.Tag.Get("bencode") == "" {
			if v.Kind() == reflect.Ptr {
				if v.IsNil() {
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
			wrapStruct2Map(v, m)
		}
	}

	for i := 0; i < val.NumField(); i++ {
		var name string
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		tag, ok := f.Tag.Lookup("bencode")
		if f.Anonymous && !ok {
			continue
		}
		v := val.FieldByIndex(f.Index)
		name = f.Name
		if ok {
			key, opts := parseTag(tag)
			if opts.Ignored() {
				continue
			}
			if key != "" {
				name = key
			}
		}
		m[name] = v
	}
}

func (dec *Decoder) readValueInto(buf *bytes.Buffer) error {
	next, err := dec.peekByte()
	if err != nil {
		return err
	}

	switch next {
	case 'i':
		return dec.readIntInto(buf)
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return dec.readStringInto(buf)
	case 'l':
		return dec.readListInto(buf)
	case 'd':
		return dec.readDictInto(buf)
	default:
		return &SyntaxError{"Invalid input", int64(dec.offset)}
	}
}

func (dec *Decoder) readIntInto(buf *bytes.Buffer) error {
	line, err := dec.readBytes('e')
	if err != nil {
		return err
	}
	if buf != nil {
		_, err = buf.Write(line)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) readStringInto(buf *bytes.Buffer) error {
	line, err := dec.readBytes(':')
	if err != nil {
		return err
	}
	if buf != nil {
		_, err = buf.Write(line)
		if err != nil {
			return err
		}
	}
	length, err := strconv.ParseInt(string(line[:len(line)-1]), 10, 64)
	if buf != nil {
		_, err = dec.readNBytesInto(buf, length)
		if err != nil {
			return err
		}
	} else {
		_, err := dec.discard(int(length))
		if err != nil {
			return err
		}
	}
	return nil
}

func (dec *Decoder) readListInto(buf *bytes.Buffer) error {
	b, err := dec.readByte()
	if err != nil {
		return err
	}
	if b != 'l' {
		panic("got something other than a list head after a peek")
	}

	if buf != nil {
		err = buf.WriteByte(b)
		if err != nil {
			return err
		}
	}

	for {
		ch, err := dec.peekByte()
		if err != nil {
			return err
		}
		if ch == 'e' {
			_, err = dec.readByte()
			if err != nil {
				return err
			}
			if buf != nil {
				return buf.WriteByte('e')
			}
			return nil
		}
		err = dec.readValueInto(buf)
		if err != nil {
			return err
		}
	}
}

func (dec *Decoder) readDictInto(buf *bytes.Buffer) error {
	b, err := dec.readByte()
	if err != nil {
		return err
	}
	if b != 'd' {
		panic("got something other than a list head after a peek")
	}

	if buf != nil {
		err = buf.WriteByte(b)
		if err != nil {
			return err
		}
	}
	for {
		ch, err := dec.peekByte()
		if err != nil {
			return err
		}
		if ch == 'e' {
			_, err = dec.readByte()
			if err != nil {
				return err
			}
			if buf != nil {
				return buf.WriteByte('e')
			}
			return nil
		}
		err = dec.readStringInto(buf)
		if err != nil {
			return err
		}
		err = dec.readValueInto(buf)
		if err != nil {
			return err
		}
	}
}

func (dec *Decoder) readBytes(delim byte) (line []byte, err error) {
	line, err = dec.r.ReadBytes(delim)
	dec.offset += len(line)
	return
}

func (dec *Decoder) readByte() (b byte, err error) {
	b, err = dec.r.ReadByte()
	dec.offset++
	return
}

func (dec *Decoder) readFull(p []byte) (n int, err error) {
	n, err = io.ReadFull(dec.r, p)
	dec.offset += n
	return
}

func (dec *Decoder) readNBytesInto(dst io.Writer, n int64) (written int64, err error) {
	written, err = io.CopyN(dst, dec.r, n)
	dec.offset += int(written)
	return
}

func (dec *Decoder) peekByte() (b byte, err error) {
	ch, err := dec.r.Peek(1)
	if err != nil {
		return 0, err
	}
	b = ch[0]
	return
}

func (dec *Decoder) discard(n int) (int, error) {
	x, err := dec.r.Discard(n)
	dec.offset += x
	return x, err
}
