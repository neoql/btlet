package bencode

import (
	"bytes"
	"encoding"
	"fmt"
	"io"
	"reflect"
	"sort"
)

// Marshal returns the bencode of v.
func Marshal(v interface{}) ([]byte, error) {
	buf := bytes.Buffer{}
	e := NewEncoder(&buf)
	err := e.Encode(v)
	return buf.Bytes(), err
}

// Marshaler is the interface implemented by types
// that can marshal themselves into valid bencode.
type Marshaler interface {
	MarshalBencode() ([]byte, error)
}

// An Encoder writes bencoded objects to an output stream.
type Encoder struct {
	w io.Writer
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w}
}

//Encode writes the bencoded data of val to its output stream.
//If an encountered value implements the Marshaler interface,
//its MarshalBencode method is called to produce the bencode output for this value.
//If no MarshalBencode method is present but the value implements encoding.TextMarshaler instead,
//its MarshalText method is called, which encodes the result as a bencode string.
//See the documentation for Decode about the conversion of Go values to
//bencoded data.
func (e *Encoder) Encode(val interface{}) error {
	return e.encode(reflect.ValueOf(val))
}

func (e *Encoder) encode(val reflect.Value) error {
	v := val

	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		v = v.Addr()
	}

	for {
		flag := true
		switch v.Kind() {
		case reflect.Ptr, reflect.Interface:
			flag = false
			if v.IsNil() {
				return nil
			}
		}
		iface := v.Interface()
		switch vv := iface.(type) {
		case Marshaler:
			data, err := vv.MarshalBencode()
			if err != nil {
				return err
			}
			_, err = e.w.Write(data)
			if err != nil {
				return err
			}
			return nil
		case encoding.TextMarshaler:
			data, err := vv.MarshalText()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(e.w, "%d:%s", len(data), data)
			if err != nil {
				return err
			}
			return nil
		}
		if flag {
			break
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err := fmt.Fprintf(e.w, "i%de", v.Int())
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err := fmt.Fprintf(e.w, "i%de", v.Uint())
		return err
	case reflect.Bool:
		i := 0
		if v.Bool() {
			i = 1
		}
		_, err := fmt.Fprintf(e.w, "i%de", i)
		return err
	case reflect.String:
		s := v.String()
		_, err := fmt.Fprintf(e.w, "%d:%s", len(s), s)
		return err
	case reflect.Slice, reflect.Array:
		return e.encodeArrayOrSlice(v)
	case reflect.Map:
		return e.encodeMap(v)
	case reflect.Struct:
		return e.encodeStruct(v)
	}

	return &UnsupportedTypeError{v.Type()}
}

func (e *Encoder) encodeArrayOrSlice(val reflect.Value) error {
	// handle byte slices like strings
	if byteSlice, ok := val.Interface().([]byte); ok {
		_, err := fmt.Fprintf(e.w, "%d:", len(byteSlice))
		if err == nil {
			_, err = e.w.Write(byteSlice)
		}
		return err
	}

	if _, err := fmt.Fprint(e.w, "l"); err != nil {
		return err
	}

	for i := 0; i < val.Len(); i++ {
		if err := e.encode(val.Index(i)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(e.w, "e")
	return err
}

func (e *Encoder) encodeMap(val reflect.Value) error {
	if _, err := fmt.Fprint(e.w, "d"); err != nil {
		return err
	}
	var keys sortValues = val.MapKeys()
	sort.Sort(keys)

	for _, key := range keys {
		mval := val.MapIndex(key)
		mval = getDirectValue(mval)
		if !mval.IsValid() {
			continue
		}
		err := e.encode(key)
		if err != nil {
			return err
		}
		err = e.encode(mval)
		if err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(e.w, "e")
	return err
}

func (e *Encoder) encodeStruct(val reflect.Value) error {
	if _, err := fmt.Fprint(e.w, "d"); err != nil {
		return err
	}

	dict := make(dictionary, 0, val.NumField())
	dict, err := readStruct(dict, val)
	if err != nil {
		return err
	}

	sort.Sort(dict)
	for _, entry := range dict {
		_, err := fmt.Fprintf(e.w, "%d:%s", len(entry.key), entry.key)
		if err != nil {
			return err
		}
		err = e.encode(entry.value)
		if err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(e.w, "e"); err != nil {
		return err
	}
	return nil
}

func readStruct(dict dictionary, v reflect.Value) (dictionary, error) {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		
		_, ok := f.Tag.Lookup("bencode")
		if f.Anonymous && !ok {
			val := v.FieldByIndex(f.Index)
			val = getDirectValue(val)
			if !val.IsValid() {
				continue
			}
			var err error
			dict, err = readStruct(dict, val)
			if err != nil {
				return dict, err
			}
		}
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := f.Name
		if f.PkgPath != "" {
			continue
		}
		
		tag, ok := f.Tag.Lookup("bencode")
		if f.Anonymous && !ok {
			continue
		}

		val := v.FieldByIndex(f.Index)
		isEmpty := isEmptyValue(val)
		val = getDirectValue(val)
		if !val.IsValid() {
			continue
		}

		if !ok {
			dict = append(dict, pair{key, val})
			continue
		}
		name, opts := parseTag(tag)
		if opts.Ignored() {
			continue
		}
		if opts.OmitEmpty() && isEmpty {
			continue
		}
		if name != "" {
			dict = append(dict, pair{name, val})
		} else {
			dict = append(dict, pair{key, val})
		}
	}
	return dict, nil
}

type sortValues []reflect.Value

func (p sortValues) Len() int           { return len(p) }
func (p sortValues) Less(i, j int) bool { return p[i].String() < p[j].String() }
func (p sortValues) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func getDirectValue(val reflect.Value) reflect.Value {
	v := val
	for {
		k := v.Kind()
		switch k {
		case reflect.Interface, reflect.Ptr:
			if !v.IsNil() {
				v = v.Elem()
				continue
			}
			return reflect.Value{}
		default:
			return v
		}
	}
}

type pair struct {
	key   string
	value reflect.Value
}

type dictionary []pair

func (d dictionary) Len() int           { return len(d) }
func (d dictionary) Less(i, j int) bool { return d[i].key < d[j].key }
func (d dictionary) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}

	return false
}
