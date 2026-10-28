package github

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIdentityEncodingTransport(t *testing.T) {
	original, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	original.Header.Set("Accept-Encoding", "gzip")
	transport := identityEncodingTransport{base: roundTripFunc(func(got *http.Request) (*http.Response, error) {
		if got.Header.Get("Accept-Encoding") != "identity" {
			t.Fatalf("encoding=%q", got.Header.Get("Accept-Encoding"))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header), Request: got}, nil
	})}
	if _, err := transport.RoundTrip(original); err != nil {
		t.Fatal(err)
	}
	if original.Header.Get("Accept-Encoding") != "gzip" {
		t.Fatal("mutated caller request")
	}
}
