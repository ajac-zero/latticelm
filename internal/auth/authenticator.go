package auth

import "net/http"

// Authenticator inspects an HTTP request for credentials and returns a
// Principal when it recognises and validates the credential.
//
// Returning (nil, nil) signals "not my credential type" so the middleware
// chain can try the next authenticator. A non-nil error means the credential
// was recognised but invalid (e.g. expired JWT, revoked API key); the chain
// stops and the request is rejected.
type Authenticator interface {
	Authenticate(r *http.Request) (*Principal, error)
}

// AuthenticatorFunc adapts a plain function into an Authenticator.
type AuthenticatorFunc func(r *http.Request) (*Principal, error)

func (f AuthenticatorFunc) Authenticate(r *http.Request) (*Principal, error) {
	return f(r)
}
