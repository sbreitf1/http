package http

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sbreitf1/errors"
	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {
	assert.Equal(t, 4, withTestServer(func(url string, f *gin.HandlerFunc) {
		client := NewClient()
		client.DisableSSLCheck = true

		t.Run("Raw GET", func(t *testing.T) {
			*f = func(c *gin.Context) {
				assert.Equal(t, "GET", c.Request.Method)
				assert.Equal(t, "/another/nice/route", c.Request.RequestURI)
				c.String(200, "yup, all good!")
			}
			response, err := client.Do(MethodGet, url+"/another/nice/route", nil)
			errors.AssertNil(t, err)
			assertResponse(t, 200, "yup, all good!", response)
		})

		t.Run("GET with headers", func(t *testing.T) {
			*f = func(c *gin.Context) {
				assert.Equal(t, "GET", c.Request.Method)
				assert.Equal(t, "/someheaders", c.Request.RequestURI)
				assert.Equal(t, []string{"Bearer 12345"}, c.Request.Header["Authorization"])
				c.String(500, "nope :(")
			}
			response, err := client.Do(MethodGet, url+"/someheaders", func(r *Request) errors.Error {
				r.Header.Add("Authorization", "Bearer 12345")
				return nil
			})
			errors.AssertNil(t, err)
			assertResponse(t, 500, "nope :(", response)
		})

		client.DefaultHeader.Add("User-Agent", "TestClient")

		t.Run("GET with default headers", func(t *testing.T) {
			*f = func(c *gin.Context) {
				assert.Equal(t, "GET", c.Request.Method)
				assert.Equal(t, "/someheaders", c.Request.RequestURI)
				assert.Equal(t, []string{"Bearer 12345"}, c.Request.Header["Authorization"])
				assert.Equal(t, []string{"TestClient"}, c.Request.Header["User-Agent"])
				c.String(403, "not you!")
			}
			response, err := client.Do(MethodGet, url+"/someheaders", func(r *Request) errors.Error {
				r.Header.Add("Authorization", "Bearer 12345")
				return nil
			})
			errors.AssertNil(t, err)
			assertResponse(t, 403, "not you!", response)
		})

		t.Run("GET with default headers only", func(t *testing.T) {
			*f = func(c *gin.Context) {
				assert.Equal(t, "GET", c.Request.Method)
				assert.Equal(t, "/someheaders", c.Request.RequestURI)
				assert.Equal(t, []string{"TestClient"}, c.Request.Header["User-Agent"])
				c.Status(404)
			}
			response, err := client.Do(MethodGet, url+"/someheaders", nil)
			errors.AssertNil(t, err)
			assertResponse(t, 404, "", response)
		})

		t.Run("Failed request callback", func(t *testing.T) {
			// this request should not be sent
			*f = nil
			_, err := client.Do(MethodGet, url+"/gocrazy", func(r *Request) errors.Error {
				return errors.GenericError.Make()
			})
			errors.Assert(t, errors.GenericError, err)
		})

		//TODO test POST
	}))
}

func assertResponse(t *testing.T, expectedCode int, expectedBody string, r *Response) bool {
	if !assert.Equal(t, expectedCode, r.StatusCode) {
		return false
	}
	if !assert.Equal(t, int64(len(expectedBody)), r.ContentLength) {
		return false
	}
	if r.ContentLength > 0 {
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		if !assert.Equal(t, expectedBody, string(data)) {
			return false
		}
	}
	return true
}

func withTestServer(f func(url string, f *gin.HandlerFunc)) int {
	var handler gin.HandlerFunc
	requestCount := 0
	e := gin.New()
	e.Any("*route", func(c *gin.Context) {
		handler(c)
		requestCount++
	})

	port := os.Getenv("TEST_HTTP_PORT")
	if len(port) == 0 {
		port = "8080"
	}

	server := &http.Server{Addr: ":" + port, Handler: e}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				panic(err)
			}
		}
	}()
	time.Sleep(100 * time.Millisecond)

	defer func() {
		context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(context)
	}()

	f("http://localhost:"+port, &handler)

	return requestCount
}
