package bencode

import (
	"bytes"
	"sort"
	"errors"
	"fmt"
	"strconv"
)

// Decode decodes a bencoded string to string, int, list or map.
func Decode(data []byte) (obj interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()
	obj, _ = deocdeObj(data)
	return
}

// Encode encodes a string, int, dict or list value to a bencoded byte slice.
func Encode(obj interface{}) (data []byte, err error) {
	switch x := obj.(type) {
	case int:
		return encodeInt(x)
	case string:
		return encodeString(x)
	case []interface{}:
		return encodeList(x)
	case map[string]interface{}:
		return encodeDict(x)
	default:
		err = errors.New("obj must be int, string, []interface or map[string]interface{}")
	}
	return
}

func encodeInt(x int) (data []byte, err error) {
	data = append(data, 'i')
	data = append(data, []byte(strconv.Itoa(x))...)
	data = append(data, 'e')
	return
}

func encodeString(x string) (data []byte, err error) {
	data = append(data, []byte(strconv.Itoa(len(x)))...)
	data = append(data, ':')
	data = append(data, []byte(x)...)
	return
}

func encodeList(x []interface{}) (data []byte, err error) {
	var buf []byte
	data = append(data, 'l')
	for _, obj := range x {
		buf, err = Encode(obj)
		if err != nil {
			return
		}
		data = append(data, buf...)
	}
	data = append(data, 'e')
	return
}

func encodeDict(x map[string]interface{}) (data []byte, err error) {
	data = append(data, 'd')
	var keys []string
	var buf []byte
	for k := range x {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := x[k]
		buf, err = Encode(k)
		if err != nil {
			return
		}
		data = append(data, buf...)
		buf, err = Encode(v)
		if err != nil {
			return
		}
		data = append(data, buf...)
	}

	data = append(data, 'e')
	return
}

func deocdeObj(data []byte) (obj interface{}, remian []byte) {
	switch data[0] {
	case 'i':
		return decodeInt(data[1:])
	case 'l':
		return decodeList(data[1:])
	case 'd':
		return decodeDict(data[1:])
	default:
		if data[0] >= '0' && data[0] <= '9' {
			return decodeString(data)
		}
		panic(fmt.Errorf("'%c' is wrong type", data[0]))
	}
}

func decodeInt(data []byte) (x int, remain []byte) {
	end := bytes.IndexByte(data, 'e')
	x, err := strconv.Atoi(string(data[0:end]))
	if err != nil {
		panic(err)
	}
	remain = data[end+1:]
	return
}

func decodeString(data []byte) (x string, remain []byte) {
	i := bytes.IndexByte(data, ':')
	length, err := strconv.Atoi(string(data[0:i]))
	if err != nil {
		return
	}
	x = string(data[i+1 : i+1+length])
	remain = data[i+1+length:]
	return
}

func decodeList(data []byte) (x []interface{}, remain []byte) {
	var item interface{}
	for data[0] != 'e' {
		item, data = deocdeObj(data)
		x = append(x, item)
	}
	remain = data[1:]
	return
}

func decodeDict(data []byte) (x map[string]interface{}, remain []byte) {
	x = make(map[string]interface{})
	var k, v interface{}
	for data[0] != 'e' {
		k, data = deocdeObj(data)
		v, data = deocdeObj(data)
		x[k.(string)] = v
	}
	remain = data[1:]
	return
}
