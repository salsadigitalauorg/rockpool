package kube

import (
	"errors"
	"testing"
)

func TestSummariseKubectlError(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips klog discovery retries",
			in: `E0916 14:37:17.916593    2835 memcache.go:381] "Couldn't get current server API group list" err="connection refused"
E0916 14:37:17.917476    2835 memcache.go:381] "Couldn't get current server API group list" err="connection refused"
The connection to the server localhost:8080 was refused - did you specify the right host or port?`,
			want: "The connection to the server localhost:8080 was refused - did you specify the right host or port?",
		},
		{
			name: "single line passes through",
			in:   "error: You must be logged in to the server (Unauthorized)",
			want: "error: You must be logged in to the server (Unauthorized)",
		},
		{
			name: "trailing whitespace ignored",
			in:   "something went wrong\n\n",
			want: "something went wrong",
		},
		{
			name: "falls back to full text when only klog lines present",
			in:   `E0916 14:37:17.916593    2835 memcache.go:381] "Couldn't get list" err="refused"`,
			want: `E0916 14:37:17.916593    2835 memcache.go:381] "Couldn't get list" err="refused"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summariseKubectlError(errors.New(tt.in))
			if got != tt.want {
				t.Errorf("summariseKubectlError()\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}
