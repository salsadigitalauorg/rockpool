package rockpool

import (
	"context"
	"net/http"
)

// Interceptor creates an HTTP interceptor which allows us to modify requests.
// Ref: https://clavinjune.dev/en/blogs/golang-http-client-interceptors/
type Interceptor struct {
	core http.RoundTripper
}

func (Interceptor) modifyRequest(r *http.Request) *http.Request {
	req := r.Clone(context.Background())
	req.URL.Host = "127.0.0.1"
	return req
}

func (i Interceptor) RoundTrip(r *http.Request) (*http.Response, error) {
	defer func() {
		_ = r.Body.Close()
	}()

	// modify before the request is sent
	newReq := i.modifyRequest(r)

	// send the request using the DefaultTransport
	return i.core.RoundTrip(newReq)
}
