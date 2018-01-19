package bencode

import (
	"testing"
)

func TestDecode(t *testing.T) {
	checkDecodeInt(t)
	checkDecodeString(t)
	checkDecodeList(t)
	checkDecodeDict(t)
}

func checkDecodeInt(t *testing.T) {
	x, err := Decode([]byte("i123e"))
	if err != nil {
		t.Fatal(err)
	}
	if x != 123 {
		t.Fatal("decode int error")
	}
}

func checkDecodeString(t *testing.T) {
	x, err := Decode([]byte("11:Hello World"))
	if err != nil {
		t.Fatal(err)
	}
	if x != "Hello World" {
		t.Fatal("decode string error")
	}
}

func checkDecodeList(t *testing.T) {
	x, err := Decode([]byte("l3:Tom4:Jack5:Marrye"))
	if err != nil {
		t.Fatal(err)
	}

	l := x.([]interface{})
	if l[0] != "Tom" || l[1] != "Jack" || l[2] != "Marry" {
		t.Fatal("decode list error")
	}
}

func checkDecodeDict(t *testing.T) {
	x, err := Decode([]byte("d4:name3:Tom3:agei20e7:friendsl4:Jack5:Marryee"))

	if err != nil {
		t.Fatal(err)
	}

	d := x.(map[string]interface{})
	if d["name"] != "Tom" {
		t.Fatal("decode dict error, value type is string")
	}

	if d["age"] != 20 {
		t.Fatal("decode dict error, value type is int")
	}

	friends := d["friends"].([]interface{})
	if friends[0] != "Jack" || friends[1] != "Marry" {
		t.Fatal("decode dict error, value type is list")
	}
}

func TestEncode(t *testing.T) {
	checkEncodeInt(t)
	checkEncodeString(t)
	checkEncodeList(t)
	checkEncodeDict(t)
}

func checkEncodeInt(t *testing.T) {
	x := 100
	s, err := Encode(x)
	if err != nil {
		t.Fatal(err)
	}

	if string(s) != "i100e" {
		t.Fatal("encode int error")
	}
}

func checkEncodeString(t *testing.T) {
	x := "hello"
	s, err := Encode(x)
	if err != nil {
		t.Fatal(err)
	}

	if string(s) != "5:hello" {
		t.Fatal("encode string error")
	}
}

func checkEncodeList(t *testing.T) {
	a := []interface{}{1, 2, 3}
	s, err := Encode(a)
	if err != nil {
		t.Fatal(err)
	}

	if string(s) != "li1ei2ei3ee" {
		t.Log(string(s))
		t.Fatal("encode int array error")
	}

	b := []interface{}{"Tom", "Jack", "Marry"}
	s, err = Encode(b)
	if err != nil {
		t.Fatal(err)
	}

	if string(s) != "l3:Tom4:Jack5:Marrye" {
		t.Fatal("encode string array error")
	}
}

func checkEncodeDict(t *testing.T) {
	x := map[string]interface{}{
		"name":		"Tom",
		"age":  	20,
		"friends":	[]interface{}{
			"Jack", "Marry",
		},
	}

	s, err := Encode(x)
	if err != nil {
		t.Fatal(err)
	}
	
	if string(s) != "d4:name3:Tom3:agei20e7:friendsl4:Jack5:Marryee" {
		t.Error("encode dict error")
	}
}
