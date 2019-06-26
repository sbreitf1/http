package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sbreitf1/errors"
	log "github.com/sirupsen/logrus"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

var (
	// ErrGraceShutdown is returned when the server has been gracefully shut down.
	ErrGraceShutdown = errors.New("Server gracefully shut down")
	// ErrServeFailed occurs when an error occurs during http serving.
	ErrServeFailed = errors.New("Serving failed")
)

// Service defines functionality for web services that can be served.
type Service interface {
	RegisterRoutes(*gin.Engine)
	BeginServing()
	StopServing()
	Healthy() errors.Error
	Ready() errors.Error
}

// ServerConfig contains all web server specific configuration parameters.
type ServerConfig struct {
	ListenAddress string `json:"listenAddress"`
	SubSystemName string `json:"subsystemName,omitempty"`
}

// Server contains http web server functionality with kubernetes probes and prometheus metrics. It can serve an arbitrary collection of services.
type Server struct {
	config ServerConfig

	engine      *gin.Engine
	asyncServer *http.Server

	services map[string]Service
}

// NewServer returns a new instance of Server to handle web requests.
func NewServer(config *ServerConfig) (*Server, errors.Error) {
	engine := gin.New()
	server := &Server{*config, engine, nil, make(map[string]Service, 0)}

	// global middlewares
	engine.Use(ginLogger)

	// metrics
	p := ginprometheus.NewPrometheus(config.SubSystemName)
	p.ReqCntURLLabelMappingFn = func(c *gin.Context) string {
		url := c.Request.URL.String()
		for _, p := range c.Params {
			if p.Key == "id" {
				url = strings.Replace(url, p.Value, ":id", 1)
				break
			}
		}
		return url
	}
	p.Use(engine)

	// server specific routes
	engine.GET("/healthz", server.handleGetHealthz)
	engine.GET("/readiness", server.handleGetReadiness)

	return server, nil
}

// RegisterService registers a new named service in the server. The name is used to identify the server in probes.
func (server *Server) RegisterService(name string, s Service) errors.Error {
	s.RegisterRoutes(server.engine)
	server.services[name] = s
	return nil
}

// Run executes the server and gracefully shuts it down when a system signal is received.
func (server *Server) Run() errors.Error {
	// graceful shutdown: https://github.com/gin-gonic/examples/blob/master/graceful-shutdown/graceful-shutdown/server.go
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, os.Kill)

	var returnErr errors.Error
	callback := func(err errors.Error) {
		returnErr = err
		close(quit)
		if !errors.InstanceOf(err, ErrGraceShutdown) {
			log.Fatalf("Server error: %s", err)
		}
	}

	if err := server.RunAsync(callback); err != nil {
		return err
	}

	sig := <-quit
	log.Infof("Signal %v received -> Shutdown server", sig)
	if err := server.Shutdown(); err != nil {
		return err
	}
	return returnErr
}

// RunAsync begins asynchronuous handling of incoming http requests. Use Shutdown() to gracefully shut down the sever.
func (server *Server) RunAsync(callback func(errors.Error)) errors.Error {
	server.asyncServer = &http.Server{Addr: server.config.ListenAddress, Handler: server.engine}
	var returnErr errors.Error
	go func() {
		server.notifyBeginServing()
		err := server.asyncServer.ListenAndServe()
		if err != nil {
			if err == http.ErrServerClosed {
				returnErr = ErrGraceShutdown.Make()
			} else {
				returnErr = ErrServeFailed.Make().Cause(err)
			}
		}
		server.notifyStopServing()
		if callback != nil {
			callback(returnErr)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	return returnErr
}

func (server *Server) notifyBeginServing() {
	for _, service := range server.services {
		service.BeginServing()
	}
}

func (server *Server) notifyStopServing() {
	for _, service := range server.services {
		service.StopServing()
	}
}

// Shutdown gracefully stops the http server.
func (server *Server) Shutdown() errors.Error {
	// graceful shutdown: https://github.com/gin-gonic/examples/blob/master/graceful-shutdown/graceful-shutdown/server.go
	context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return errors.Wrap(server.asyncServer.Shutdown(context))
}

func ginLogger(c *gin.Context) {
	t := time.Now()

	func() {
		defer func() {
			if r := recover(); r != nil {
				err := errors.New("Recovered from panic: %v", r).MakeTraced(2)
				err.ToRequestAndLog(c)
			}
		}()

		// process remaining handlers to obtain the final response state
		c.Next()
	}()

	url := c.Request.RequestURI
	if !strings.HasPrefix(url, "/healthz") && !strings.HasPrefix(url, "/readiness") && !strings.HasPrefix(url, "/metrics") {
		str := fmt.Sprintf("%s - %d - %s - %s (%s)", c.Request.RemoteAddr, c.Writer.Status(), c.Request.Method, c.Request.RequestURI, time.Since(t))
		log.WithField("component", "gin").Info(str)
	}
}

type serviceError struct {
	ServiceName string `json:"service"`
	Message     string `json:"message"`
}

// HandleGetHealthz returns 200 OK if all registered services alive, otherwise 500.
func (server *Server) handleGetHealthz(c *gin.Context) {
	errs := make([]serviceError, 0)
	for name, service := range server.services {
		if err := service.Healthy(); err != nil {
			errs = append(errs, serviceError{name, err.Error()})
		}
	}
	if len(errs) > 0 {
		c.JSON(500, errs)
	} else {
		c.JSON(200, errs)
	}
}

// HandleGetReadiness returns 200 OK if all services are ready to serve traffic, otherwise 503.
func (server *Server) handleGetReadiness(c *gin.Context) {
	errs := make([]serviceError, 0)
	for name, service := range server.services {
		if err := service.Ready(); err != nil {
			errs = append(errs, serviceError{name, err.Error()})
		}
	}
	if len(errs) > 0 {
		c.JSON(503, errs)
	} else {
		c.JSON(200, errs)
	}
}
