package bencode

import (
	"strings"
)

type tagOptions []string

func parseTag(tag string) (string, tagOptions) {
	x := strings.Split(tag, ",")
	if len(x) == 1 {
		if x[0] == "-" {
			return "", tagOptions{"-"}
		}
		return x[0], tagOptions(nil)
	}
	return x[0], tagOptions(x[1:])
}

func (opts tagOptions) HasOpt(opt string) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

func (opts tagOptions) Ignored() bool {
	return opts.HasOpt("-")
}

func (opts tagOptions) OmitEmpty() bool {
	return opts.HasOpt("omitempty")
}
