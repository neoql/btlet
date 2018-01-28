package tools

import (
	"testing"
)

func TestCommonPrefixLen(t *testing.T) {
	a := string([]byte{0xFF, 0xF0})
	b := string([]byte{0xFF, 0xF1})

	if dis := CommonPrefixLen(a, b); dis != 15 {
		t.Fatal(dis)
	}

	c := string([]byte{0xFF, 0xF8})
	d := string([]byte{0xFF, 0xF4})

	if dis := CommonPrefixLen(c, d); dis != 12 {
		t.Fatal(dis)
	}
}

func TestDecodeCompactIPPortInfo(t *testing.T) {
	cases := []struct {
		in  string
		out struct {
			ip   string
			port int
		}
	}{
		{"123456", struct {
			ip   string
			port int
		}{"49.50.51.52", 13622}},
		{"abcdef", struct {
			ip   string
			port int
		}{"97.98.99.100", 25958}},
	}

	for _, item := range cases {
		ip, port, err := DecodeCompactIPPortInfo(item.in)
		if err != nil || ip.String() != item.out.ip || port != item.out.port {
			t.Fail()
		}
	}
}
