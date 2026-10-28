package github

import (
	"net/http"
	"time"
)

// IdentityEncodingClient prevents compressed GitHub responses. Novita's
// brokered-credential filter currently rejects some compressed upstream
// responses before they reach the controller. Identity encoding is harmless
// in local mode and required in hosted mode.
func IdentityEncodingClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	return &http.Client{
		Transport: identityEncodingTransport{base: transport},
		Timeout:   30 * time.Minute,
	}
}

type identityEncodingTransport struct{ base http.RoundTripper }

func (t identityEncodingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Accept-Encoding", "identity")
	return t.base.RoundTrip(clone)
}
