package config

import (
	"sync"

	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
)

// DispatchFn is a function that will be dispatched.
type DispatchFn func(interface{}) error

// DispatchCancelFn is a function that cancels a dispatched function.
type DispatchCancelFn func()

// Dispatcher implements a dispatcher pattern.
type Dispatcher struct {
	logger logrus.FieldLogger
	keys   map[string]DispatchFn

	mu sync.Mutex
}

// NewDispatcher creates an instance of Dispatcher.
func NewDispatcher() *Dispatcher {
	logger := logrus.WithField("component", "dispatcher")

	return &Dispatcher{
		logger: logger,
		keys:   make(map[string]DispatchFn),
	}
}

// Dispatch dispatches a value to all the watchers.
func (d *Dispatcher) Dispatch(v interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, fn := range d.keys {
		go func(fn DispatchFn) {
			if err := fn(v); err != nil {
				d.logger.WithError(err).Error("dispatching to function")
			}
		}(fn)
	}
}

// Watch configures a watcher.
func (d *Dispatcher) Watch(fn DispatchFn) DispatchCancelFn {
	d.mu.Lock()
	defer d.mu.Unlock()

	u := uuid.Must(uuid.NewV4())
	d.keys[u.String()] = fn

	cancel := func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		delete(d.keys, u.String())

		return
	}

	return cancel
}
