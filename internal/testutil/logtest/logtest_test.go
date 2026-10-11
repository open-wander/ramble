package logtest

import (
	"bytes"
	"testing"
)

func TestLinesAndFind(t *testing.T) {
	var empty bytes.Buffer
	if got := Lines(t, &empty); got != nil {
		t.Errorf("empty buffer: got %v", got)
	}
	buf := bytes.NewBufferString("{\"message\":\"a\",\"n\":1}\n{\"message\":\"b\"}\n")
	if got := Lines(t, buf); len(got) != 2 {
		t.Fatalf("got %d lines", len(got))
	}
	if m := Find(t, buf, "b"); m == nil || m["message"] != "b" {
		t.Errorf("Find b = %v", m)
	}
	if m := Find(t, buf, "zzz"); m != nil {
		t.Errorf("Find zzz = %v", m)
	}
}
