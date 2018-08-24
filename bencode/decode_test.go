package bencode

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDecode(t *testing.T) {
	type testCase struct {
		in                string
		val               interface{}
		expect            interface{}
		err               bool
		unorderedFail     bool
		unknownFieldsFail bool
	}

	type dT struct {
		X string
		Y int
		Z string `bencode:"zff"`
	}

	type dT2 struct {
		A string
		B string `bencode:","`
	}

	type Embedded struct {
		B string
	}

	type discardNonFieldDef struct {
		B string
		D string
	}

	type tagCover struct {
		A  string
		A1 string `bencode:"A"`
	}

	type withUnmarshalerField struct {
		X     string       `bencode:"x"`
		Time  myTimeType   `bencode:"t"`
		Foo   myBoolType   `bencode:"f"`
		Bar   myStringType `bencode:"b"`
		Slice mySliceType  `bencode:"s"`
		Y     string       `bencode:"y"`
	}

	type withTextUnmarshalerField struct {
		X     string           `bencode:"x"`
		Foo   myBoolTextType   `bencode:"f"`
		Bar   myTextStringType `bencode:"b"`
		Slice myTextSliceType  `bencode:"s"`
		Y     string           `bencode:"y"`
	}

	type withErrUnmarshalerField struct {
		Name  string           `bencode:"n"`
		Error errorMarshalType `bencode:"e"`
	}

	type withErrTextUnmarshalerField struct {
		Name  string               `bencode:"n"`
		Error errorTextMarshalType `bencode:"e"`
	}

	now := time.Now()

	var decodeCases = []testCase{
		//integers
		{`i5e`, new(int), int(5), false, false, false},
		{`i-10e`, new(int), int(-10), false, false, false},
		{`i8e`, new(uint), uint(8), false, false, false},
		{`i8e`, new(uint8), uint8(8), false, false, false},
		{`i8e`, new(uint16), uint16(8), false, false, false},
		{`i8e`, new(uint32), uint32(8), false, false, false},
		{`i8e`, new(uint64), uint64(8), false, false, false},
		{`i8e`, new(int), int(8), false, false, false},
		{`i8e`, new(int8), int8(8), false, false, false},
		{`i8e`, new(int16), int16(8), false, false, false},
		{`i8e`, new(int32), int32(8), false, false, false},
		{`i8e`, new(int64), int64(8), false, false, false},
		{`i0e`, new(*int), new(int), false, false, false},
		{`i-2e`, new(uint), nil, true, false, false},

		//bools
		{`i1e`, new(bool), true, false, false, false},
		{`i0e`, new(bool), false, false, false, false},
		{`i0e`, new(*bool), new(bool), false, false, false},
		{`i8e`, new(bool), true, false, false, false},

		//strings
		{`3:foo`, new(string), "foo", false, false, false},
		{`4:foob`, new(string), "foob", false, false, false},
		{`0:`, new(*string), new(string), false, false, false},
		{`6:short`, new(string), nil, true, false, false},

		//lists
		{`l3:foo3:bare`, new([]string), []string{"foo", "bar"}, false, false, false},
		{`li15ei20ee`, new([]int), []int{15, 20}, false, false, false},
		{`ld3:fooi0eed3:bari1eee`, new([]map[string]int), []map[string]int{
			{"foo": 0},
			{"bar": 1},
		}, false, false, false},

		//dicts
		{`d3:foo3:bar4:foob3:fooe`, new(map[string]string), map[string]string{
			"foo":  "bar",
			"foob": "foo",
		}, false, false, false},
		{`d1:X3:foo1:Yi10e3:zff3:bare`, new(dT), dT{"foo", 10, "bar"}, false, false, false},
		// encoding/bencode takes, if set, the tag as name and doesn't falls back to the
		// struct field's name.
		{`d1:X3:foo1:Yi10e1:Z3:bare`, new(dT), dT{"foo", 10, ""}, false, false, false},
		// test DisallowUnknownFields
		{`d1:X3:foo1:Yi10e1:h3:bare`, new(dT), dT{"foo", 10, ""}, false, false, false},
		{`d1:X3:foo1:Yi10e1:h3:bare`, new(dT), dT{"foo", 10, ""}, true, false, true},

		{`d3:fooli0ei1ee3:barli2ei3eee`, new(map[string][]int), map[string][]int{
			"foo": []int{0, 1},
			"bar": []int{2, 3},
		}, false, false, false},
		{`de`, new(map[string]string), map[string]string{}, false, false, false},

		//into interfaces
		{`i5e`, new(interface{}), int64(5), false, false, false},
		{`li5ee`, new(interface{}), []interface{}{int64(5)}, false, false, false},
		{`5:hello`, new(interface{}), "hello", false, false, false},
		{`d5:helloi5ee`, new(interface{}), map[string]interface{}{"hello": int64(5)}, false, false, false},

		//into values whose type support the Unmarshaler interface
		{`1:y`, new(myTimeType), nil, true, false, false},
		{fmt.Sprintf("i%de", now.Unix()), new(myTimeType), myTimeType{time.Unix(now.Unix(), 0)}, false, false, false},
		{`1:y`, new(myBoolType), myBoolType(true), false, false, false},
		{`i42e`, new(myBoolType), nil, true, false, false},
		{`1:n`, new(myBoolType), myBoolType(false), false, false, false},
		{`1:n`, new(errorMarshalType), nil, true, false, false},
		{`li102ei111ei111ee`, new(myStringType), myStringType("foo"), false, false, false},
		{`i42e`, new(myStringType), nil, true, false, false},
		{`d1:ai1e3:foo3:bare`, new(mySliceType), mySliceType{"a", int64(1), "foo", "bar"}, false, false, false},
		{`i42e`, new(mySliceType), nil, true, false, false},
		//into values who have a child which type supports the Unmarshaler interface
		{
			fmt.Sprintf(`d1:b3:foo1:f1:y1:sd1:f3:foo1:ai42ee1:ti%de1:x1:x1:y1:ye`, now.Unix()),
			new(withUnmarshalerField),
			withUnmarshalerField{
				X:     "x",
				Time:  myTimeType{time.Unix(now.Unix(), 0)},
				Foo:   myBoolType(true),
				Bar:   myStringType("foo"),
				Slice: mySliceType{"a", int64(42), "f", "foo"},
				Y:     "y",
			},
			false,
			false,
			false,
		},
		{
			`d1:ei42e1:n3:fooe`,
			new(withErrUnmarshalerField),
			nil,
			true,
			false,
			false,
		},

		//into values whose type support the TextUnmarshaler interface
		{`1:y`, new(myBoolTextType), myBoolTextType(true), false, false, false},
		{`1:n`, new(myBoolTextType), myBoolTextType(false), false, false, false},
		{`i42e`, new(myBoolTextType), nil, true, false, false},
		{`1:n`, new(errorTextMarshalType), nil, true, false, false},
		{`7:foo_bar`, new(myTextStringType), myTextStringType("bar"), false, false, false},
		{`i42e`, new(myTextStringType), nil, true, false, false},
		{`7:a,b,c,d`, new(myTextSliceType), myTextSliceType{"a", "b", "c", "d"}, false, false, false},
		{`i42e`, new(myTextSliceType), nil, true, false, false},

		//into values who have a child which type supports the TextUnmarshaler interface
		{
			`d1:b7:foo_bar1:f1:y1:s5:1,2,31:x1:x1:y1:ye`,
			new(withTextUnmarshalerField),
			withTextUnmarshalerField{
				X:     "x",
				Foo:   myBoolTextType(true),
				Bar:   myTextStringType("bar"),
				Slice: myTextSliceType{"1", "2", "3"},
				Y:     "y",
			},
			false,
			false,
			false,
		},
		{
			`d1:ei42e1:n3:fooe`,
			new(withErrTextUnmarshalerField),
			nil,
			true,
			false,
			false,
		},

		//malformed
		{`i53:foo`, new(interface{}), nil, true, false, false},
		{`6:foo`, new(interface{}), nil, true, false, false},
		{`di5ei2ee`, new(interface{}), nil, true, false, false},
		{`d3:fooe`, new(interface{}), nil, true, false, false},
		{`l3:foo3:bar`, new(interface{}), nil, true, false, false},
		{`d-1:`, new(interface{}), nil, true, false, false},

		// embedded structs
		{`d1:A3:foo1:B3:bare`, new(struct {
			A string
			Embedded
		}), struct {
			A string
			Embedded
		}{"foo", Embedded{"bar"}}, false, false, false},

		// Embedded structs with a valid tag are encoded as a definition
		{`d1:B3:bar6:nestedd1:B3:fooee`, new(struct {
			Embedded `bencode:"nested"`
		}), struct {
			Embedded `bencode:"nested"`
		}{Embedded{"foo"}}, false, false, false},

		// Don't fail when reading keys missing from the struct
		{"d1:A7:discard1:B4:take1:C7:discard1:D4:takee",
			new(discardNonFieldDef),
			discardNonFieldDef{"take", "take"},
			false,
			false,
			false,
		},
		// ail when set DisallowUnknownFields
		{"d1:A7:discard1:B4:take1:C7:discard1:D4:takee",
			new(discardNonFieldDef),
			discardNonFieldDef{"take", "take"},
			true,
			false,
			true,
		},

		// tag cover field
		{"d1:A1:a1:A1:be", new(tagCover), tagCover{"", "b"}, false, false, false},

		// Empty struct
		{"de", new(struct{}), struct{}{}, false, false, false},

		// Fail on unordered dictionaries
		{"d1:Yi10e1:X1:a3:zff1:ce", new(dT), dT{}, true, true, false},
		{"d3:zff1:c1:Yi10e1:X1:ae", new(dT), dT{}, true, true, false},

		// tag ","
		{"d1:A5:hello1:B5:worlde", new(dT2), dT2{"hello", "world"}, false, false, false},
	}

	for i, tt := range decodeCases {
		dec := NewDecoder(strings.NewReader(tt.in))
		if tt.unorderedFail {
			dec.DisallowUnorderedKeys()
		}
		if tt.unknownFieldsFail {
			dec.DisallowUnknownFields()
		}
		err := dec.Decode(tt.val)
		if !tt.err && err != nil {
			t.Fatalf("#%d (%v): Unexpected err: %v", i, tt.in, err)
			continue
		}
		if tt.err && err == nil {
			t.Fatalf("#%d (%v): Expected err is nil", i, tt.in)
			continue
		}
		if !tt.err && err == nil && dec.BytesParsed() != len(tt.in) {
			t.Fatalf("#%d (%v): BytesParsed is a wrong value", i, tt.in)
		}
		v := reflect.ValueOf(tt.val).Elem().Interface()
		if !reflect.DeepEqual(v, tt.expect) && !tt.err {
			t.Fatalf("#%d (%v): Val: %#v != %#v", i, tt.in, v, tt.expect)
		}
	}
}

func TestRawDecode(t *testing.T) {
	type testCase struct {
		in     string
		expect []byte
		err    bool
	}

	var rawDecodeCases = []testCase{
		{`i5e`, []byte(`i5e`), false},
		{`5:hello`, []byte(`5:hello`), false},
		{`li5ei10e5:helloe`, []byte(`li5ei10e5:helloe`), false},
		{`llleee`, []byte(`llleee`), false},
		{`li5eli5eli5eeee`, []byte(`li5eli5eli5eeee`), false},
		{`d5:helloi5ee`, []byte(`d5:helloi5ee`), false},
	}

	for i, tt := range rawDecodeCases {
		var x RawMessage
		err := Unmarshal([]byte(tt.in), &x)
		if !tt.err && err != nil {
			t.Fatalf("#%d: Unexpected err: %v", i, err)
			continue
		}
		if tt.err && err == nil {
			t.Fatalf("#%d: Expected err is nil", i)
			continue
		}
		if !reflect.DeepEqual(x, RawMessage(tt.expect)) && !tt.err {
			t.Fatalf("#%d: Val: %#v != %#v", i, x, tt.expect)
		}
	}
}

func TestNestedRawDecode(t *testing.T) {
	type testCase struct {
		in     string
		val    interface{}
		expect interface{}
		err    bool
	}

	type message struct {
		Key string
		Val int
		Raw RawMessage
	}

	var cases = []testCase{
		{`li5e5:hellod1:a1:beli5eee`, new([]RawMessage), []RawMessage{
			RawMessage(`i5e`),
			RawMessage(`5:hello`),
			RawMessage(`d1:a1:be`),
			RawMessage(`li5ee`),
		}, false},
		{`d1:a1:b1:c1:de`, new(map[string]RawMessage), map[string]RawMessage{
			"a": RawMessage(`1:b`),
			"c": RawMessage(`1:d`),
		}, false},
		{`d3:Key5:hello3:Rawldedei5e1:ae3:Vali10ee`, new(message), message{
			Key: "hello",
			Val: 10,
			Raw: RawMessage(`ldedei5e1:ae`),
		}, false},
	}

	for i, tt := range cases {
		err := Unmarshal([]byte(tt.in), tt.val)
		if !tt.err && err != nil {
			t.Errorf("#%d: Unexpected err: %v", i, err)
			continue
		}
		if tt.err && err == nil {
			t.Errorf("#%d: Expected err is nil", i)
			continue
		}
		v := reflect.ValueOf(tt.val).Elem().Interface()
		if !reflect.DeepEqual(v, tt.expect) && !tt.err {
			t.Errorf("#%d: Val:\n%#v !=\n%#v", i, v, tt.expect)
		}
	}
}
