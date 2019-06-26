package http

import (
	"crypto/tls"
	"net/http"

	"github.com/sbreitf1/errors"
)

// Client is used to send or mock HTTP requests.
type Client struct {
	DefaultHeader Header
	// DisableSSLCheck can be set to true, to accept invalid and self-signed certificates in HTTPS connections.
	DisableSSLCheck bool
	// RequestResponder denotes the technical implementation for sending requests. Overwrite this property to inject mocked responses.
	RequestResponder func(req *Request) (*Response, errors.Error)
}

// NewClient returns a new HTTP client to send requests.
func NewClient() *Client {
	client := &Client{DefaultHeader: make(Header)}

	client.RequestResponder = func(req *Request) (*Response, errors.Error) {
		c := &http.Client{}
		if client.DisableSSLCheck {
			c.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		}
		response, err := c.Do(req)
		return response, errors.Wrap(err)
	}

	return client
}

// Do requests the given url using the given method and returns the response. Use the callback function f to modify the request directly before sending.
func (client *Client) Do(method RequestMethod, url string, f func(*Request) errors.Error) (*Response, errors.Error) {
	req, err := http.NewRequest(method.String(), url, nil)
	if err != nil {
		return nil, ErrInvalidRequest.Make().Cause(err)
	}

	for h, values := range client.DefaultHeader {
		for _, v := range values {
			req.Header.Add(h, v)
		}
	}

	if f != nil {
		if err := f(req); err != nil {
			return nil, err
		}
	}

	response, err := client.RequestResponder(req)
	if err != nil {
		return nil, ErrRequestFailed.Make().Cause(err)
	}

	return response, nil
}
