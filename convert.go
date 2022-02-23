package gochan

import (
	"io"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

func toUTF(str string) string {
	ret, err := io.ReadAll(transform.NewReader(strings.NewReader(str), japanese.ShiftJIS.NewDecoder()))
	if err == nil {
		return string(ret)
	} else {
		return ""
	}
}

func toSJIS(str string) string {
	ret, err := io.ReadAll(transform.NewReader(strings.NewReader(str), japanese.ShiftJIS.NewEncoder()))
	if err == nil {
		return string(ret)
	} else {
		return ""
	}
}

// func stream_toUTF(r io.Reader, w io.Writer) {
// 	decoder := transform.NewReader(r, japanese.ShiftJIS.NewDecoder())
// 	io.Copy(w, decoder)
// }

func stream_toSJIS(r io.Reader, w io.Writer) {
	encoder := transform.NewReader(r, japanese.ShiftJIS.NewEncoder())
	io.Copy(w, encoder)
}
