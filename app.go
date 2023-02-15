package summer

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync/atomic"
)

// CheckerFunc health check function, see [App.Check]
type CheckerFunc func(ctx context.Context) (err error)

// HandlerFunc handler func with [Context] as argument
type HandlerFunc[T Context] func(ctx T)

// App the main interface of [summer]
type App[T Context] interface {
	http.Handler

	// CheckFunc register a checker function with given name
	//
	// Invoking '/debug/ready' will evaluate all registered checker functions
	CheckFunc(name string, fn CheckerFunc)

	// HandleFunc register an action function with given path pattern
	//
	// This function is similar with [http.ServeMux.HandleFunc]
	HandleFunc(pattern string, fn HandlerFunc[T])
}

type app[T Context] struct {
	contextFactory ContextFactory[T]

	opts options

	checkers map[string]CheckerFunc

	mux *http.ServeMux

	h     http.Handler
	debug http.Handler

	cc chan struct{}

	readinessFailed int64
}

func (a *app[T]) CheckFunc(name string, fn CheckerFunc) {
	a.checkers[name] = fn
}

func (a *app[T]) executeCheckers(ctx context.Context) (r string, failed bool) {
	sb := &strings.Builder{}
	for k, fn := range a.checkers {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(k)
		sb.WriteString(": ")
		if err := fn(ctx); err == nil {
			sb.WriteString("OK")
		} else {
			failed = true
			sb.WriteString(err.Error())
		}
	}
	r = sb.String()
	if r == "" {
		r = "OK"
	}
	return
}

func (a *app[T]) HandleFunc(pattern string, fn HandlerFunc[T]) {
	a.mux.Handle(
		pattern,
		otelhttp.WithRouteTag(
			pattern,
			http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				c := a.contextFactory(rw, req)
				func() {
					defer c.Perform()
					fn(c)
				}()
			}),
		),
	)
}

func (a *app[T]) initialize() {
	// checkers
	a.checkers = map[string]CheckerFunc{}

	// debug handler
	m := &http.ServeMux{}
	m.HandleFunc(DebugPathAlive, func(rw http.ResponseWriter, req *http.Request) {
		if a.opts.readinessCascade > 0 && atomic.LoadInt64(&a.readinessFailed) > a.opts.readinessCascade {
			respondText(rw, "CASCADED", http.StatusInternalServerError)
		} else {
			respondText(rw, "OK", http.StatusOK)
		}
	})
	m.HandleFunc(DebugPathReady, func(rw http.ResponseWriter, req *http.Request) {
		r, failed := a.executeCheckers(req.Context())
		status := http.StatusOK
		if failed {
			atomic.AddInt64(&a.readinessFailed, 1)
			status = http.StatusInternalServerError
		} else {
			atomic.StoreInt64(&a.readinessFailed, 0)
		}
		respondText(rw, r, status)
	})
	m.Handle(DebugPathMetrics, promhttp.Handler())
	m.HandleFunc("/debug/pprof/", pprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)
	a.debug = m

	// handler
	a.mux = &http.ServeMux{}
	a.h = otelhttp.NewHandler(a.mux, "http")

	// concurrency control
	if a.opts.concurrency > 0 {
		a.cc = make(chan struct{}, a.opts.concurrency)
		for i := 0; i < a.opts.concurrency; i++ {
			a.cc <- struct{}{}
		}
	}
}

func (a *app[T]) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, DebugPathPrefix) {
		a.debug.ServeHTTP(rw, req)
		return
	}

	// concurrency control
	if a.cc != nil {
		<-a.cc
		defer func() {
			a.cc <- struct{}{}
		}()
	}

	a.h.ServeHTTP(rw, req)
}

// New create an [App] with optional [Option]
func New[T Context](cf ContextFactory[T], opts ...Option) App[T] {
	a := &app[T]{
		contextFactory: cf,

		opts: options{
			concurrency:      128,
			readinessCascade: 5,
		},
	}
	for _, opt := range opts {
		opt(&a.opts)
	}
	a.initialize()
	return a
}
