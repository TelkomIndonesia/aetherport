package main

import (
	"bytes"
	"fmt"
)

type Label struct {
	key   string
	value string
}

func newLabel(key string, value string) Label {
	return Label{key: key, value: value}
}

func labelFromString(s string) (l Label, err error) {
	err = l.UnmarshalText([]byte(s))
	return
}

func (l Label) String() string {
	if l.key == "" {
		return ""
	}

	return l.key + "=" + l.value
}

func (l Label) MarshalText() (text []byte, err error) {
	return []byte(l.String()), err
}

func (l *Label) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}

	i := bytes.IndexRune(text, '=')
	if i < 1 {
		return fmt.Errorf("invalid labels: %x", text)
	}

	l.key = string(text[:i-1])
	if i < len(text)-1 {
		l.value = string(text[i+1:])
	}
	return nil
}

func (l Label) MarshalBinary() (data []byte, err error) {
	return l.MarshalText()
}

func (l *Label) UnmarshalBinary(data []byte) error {
	return l.UnmarshalText(data)
}
