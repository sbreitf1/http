package http

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sbreitf1/errors"
	"github.com/stretchr/testify/assert"
)

func init() {
	//TODO just to silence gin:
	/*errors.Logger = log.Errorf
	gin.SetMode(gin.ReleaseMode)
	gin.DefaultWriter = ioutil.Discard
	gin.DefaultErrorWriter = ioutil.Discard
	log.SetOutput(ioutil.Discard)*/
}

func TestDefaultEndpoints(t *testing.T) {
	server, url := newTestServer()
	if err := server.RunAsync(nil); err != nil {
		panic(err)
	}
	defer server.Shutdown()

	t.Run("test non-existent endpoint", func(t *testing.T) {
		resp, err := http.Get(url + "/nonexistent")
		errors.AssertNil(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("test healthz", func(t *testing.T) {
		resp, err := http.Get(url + "/healthz")
		errors.AssertNil(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("test readiness", func(t *testing.T) {
		resp, err := http.Get(url + "/readiness")
		errors.AssertNil(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("test metrics", func(t *testing.T) {
		resp, err := http.Get(url + "/metrics")
		errors.AssertNil(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}

func TestUnhealthy(t *testing.T) {
	service := newTestService(t)
	service.Healthiness = errors.GenericError.Msg("poor service is ill :(").Make()
	server, url := newTestServer()
	server.RegisterService("test-service", service)
	if err := server.RunAsync(nil); err != nil {
		panic(err)
	}
	defer server.Shutdown()

	resp, err := http.Get(url + "/healthz")
	errors.AssertNil(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

func TestNotReady(t *testing.T) {
	service := newTestService(t)
	service.Readiness = errors.GenericError.Msg("wait! wait! wait!").Make()
	server, url := newTestServer()
	server.RegisterService("test-service", service)
	if err := server.RunAsync(nil); err != nil {
		panic(err)
	}
	defer server.Shutdown()

	resp, err := http.Get(url + "/readiness")
	errors.AssertNil(t, err)
	assert.Equal(t, 503, resp.StatusCode)
}

func TestServerFail(t *testing.T) {
	server1, _ := newTestServer()
	if err := server1.RunAsync(nil); err != nil {
		panic(err)
	}
	defer server1.Shutdown()
	server2, _ := newTestServer()
	assert.Error(t, server2.RunAsync(nil))
}

func TestRun(t *testing.T) {
	server, _ := newTestServer()
	var returnErr errors.Error
	go func() {
		returnErr = server.Run()
	}()
	time.Sleep(100 * time.Millisecond)
	errors.AssertNil(t, server.Shutdown())
	awaitTrue(t, func() bool { return errors.InstanceOf(returnErr, ErrGraceShutdown) }, "Expected graceful server shutdown error, but got %v instead", returnErr)
}

func TestCallbacks(t *testing.T) {
	service := newTestService(t)
	assert.False(t, service.RoutesRegistered, "Routes should not be registered yet")
	assert.False(t, service.BeginNotified, "BeginServing should not be notified yet")
	assert.False(t, service.EndNotified, "StopServing should not be notified yet")

	server, _ := newTestServer()
	server.RegisterService("test-service", service)
	assert.True(t, service.RoutesRegistered, "Routes should be registered")
	assert.False(t, service.BeginNotified, "BeginServing should not be notified yet")
	assert.False(t, service.EndNotified, "StopServing should not be notified yet")

	var returnErr errors.Error
	shutdownCallback := false
	callback := func(err errors.Error) {
		returnErr = err
		shutdownCallback = true
	}
	if err := server.RunAsync(callback); err != nil {
		panic(err)
	}
	awaitTrue(t, func() bool { return service.BeginNotified }, "BeginServing should be notified")
	assert.False(t, service.EndNotified, "StopServing should not be notified yet")

	err := server.Shutdown()
	errors.AssertNil(t, err)
	awaitTrue(t, func() bool { return service.EndNotified }, "EndServing should be notified")

	awaitTrue(t, func() bool { return shutdownCallback }, "Shutdown callback should be executed")
	assert.True(t, errors.InstanceOf(returnErr, ErrGraceShutdown), "Expected graceful server shutdown error, but got %v instead", returnErr)
}

func TestRecover(t *testing.T) {
	service := newTestService(t)
	server, url := newTestServer()
	defer server.Shutdown()
	server.RegisterService("test-service", service)
	server.RunAsync(nil)
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get(url + "/panic")
	if errors.AssertNil(t, err) {
		assert.Equal(t, 500, resp.StatusCode)
	}
}

/* ############################################# */
/* ###                Helper                 ### */
/* ############################################# */

func newTestServer() (*Server, string) {
	port := os.Getenv("TEST_HTTP_PORT")
	if len(port) == 0 {
		port = "8080"
	}
	config := ServerConfig{ListenAddress: ":" + port}
	server, err := NewServer(&config)
	if err != nil {
		panic(err)
	}
	return server, "http://localhost:" + port
}

type testService struct {
	T                          *testing.T
	RoutesRegistered           bool
	BeginNotified, EndNotified bool
	Healthiness, Readiness     errors.Error
}

func newTestService(t *testing.T) *testService {
	return &testService{t, false, false, false, nil, nil}
}

func (svc *testService) RegisterRoutes(c *gin.Engine) {
	assert.NotNil(svc.T, c)
	assert.False(svc.T, svc.RoutesRegistered, "Routes already registered")
	c.GET("/panic", svc.handleGetPanic)
	svc.RoutesRegistered = true
}
func (svc *testService) BeginServing() {
	assert.False(svc.T, svc.EndNotified, "BeginServing notified after StopServing")
	assert.False(svc.T, svc.BeginNotified, "BeginServing already notified")
	svc.BeginNotified = true
}
func (svc *testService) StopServing() {
	assert.True(svc.T, svc.BeginNotified, "StopServing notified before BeginServing")
	assert.False(svc.T, svc.EndNotified, "StopServing already notified")
	svc.EndNotified = true
}
func (svc *testService) Healthy() errors.Error {
	return svc.Healthiness
}
func (svc *testService) Ready() errors.Error {
	return svc.Readiness
}
func (svc *testService) handleGetPanic(c *gin.Context) {
	panic("human readable panic message")
}

func awaitTrue(t *testing.T, f func() bool, msgAndArgs ...interface{}) bool {
	return await(t, func(t *testing.T) bool { return assert.True(t, f(), msgAndArgs...) })
}

func await(t *testing.T, f func(t *testing.T) bool) bool {
	tmpT := &testing.T{}
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) && !f(tmpT) {
		time.Sleep(1 * time.Millisecond)
	}
	return f(t)
}
