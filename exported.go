package http

import (
	"net/http"

	"github.com/sbreitf1/errors"
)

const (
	// MethodGet specifies the HTTP GET method.
	MethodGet RequestMethod = "GET"
	// MethodHead specifies the HTTP HEAD method.
	MethodHead RequestMethod = "HEAD"
	// MethodPost specifies the HTTP POST method.
	MethodPost RequestMethod = "POST"
	// MethodPut specifies the HTTP PUT method.
	MethodPut RequestMethod = "PUT"
	// MethodDelete specifies the HTTP DELETE method.
	MethodDelete RequestMethod = "DELETE"
	// MethodConnect specifies the HTTP CONNECT method.
	MethodConnect RequestMethod = "CONNECT"
	// MethodOptions specifies the HTTP OPTIONS method.
	MethodOptions RequestMethod = "OPTIONS"
	// MethodTrace specifies the HTTP TRACE method.
	MethodTrace RequestMethod = "TRACE"
)

// RequestMethod denotes a HTTP request method.
type RequestMethod string

func (m RequestMethod) String() string {
	return string(m)
}

var (
	// DefaultClient denotes the client used for all default accessors.
	DefaultClient *Client
	// ErrInvalidRequest occurs when trying to create a new request using malformed parameters.
	ErrInvalidRequest = errors.New("Invalid request")
	// ErrRequestFailed occurs when sending a request or receiving a response failed.
	ErrRequestFailed = errors.New("Request failed")
)

// Request is an alias for the default 'net/http' Request type.
type Request = http.Request

// Response is an alias for the default 'net/http' Response type.
type Response = http.Response

// Header is an alias for the default 'net/http' Header type.
type Header = http.Header

func init() {
	DefaultClient = NewClient()
}

// Do performs a request using the default client.
func Do(method RequestMethod, url string, f func(*Request) errors.Error) (*Response, errors.Error) {
	return DefaultClient.Do(method, url, f)
}
