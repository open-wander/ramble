package server

import (
	"reflect"
	"testing"
)

func TestTrustedProxies(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"unset falls back to the private ranges", "", defaultTrustedProxies},
		{"whitespace only falls back", " , ", defaultTrustedProxies},
		{"one range", "100.64.0.0/10", []string{"100.64.0.0/10"}},
		{"list with spaces", " 127.0.0.1/8, ::1 ,100.64.0.0/10", []string{"127.0.0.1/8", "::1", "100.64.0.0/10"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trustedProxies(tt.env); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("trustedProxies(%q) = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}
