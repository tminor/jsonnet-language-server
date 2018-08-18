package server

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
)

const (
	// CfgJsonnetLibPaths are jsonnet lib paths.
	CfgJsonnetLibPaths = "jsonnet.libPaths"
)

// Config is configuration setting for the server.
type Config struct {
	// JsonnetLibPaths are jsonnet lib paths.
	JsonnetLibPaths []string

	dispatchers map[string]*Dispatcher
}

// NewConfig creates an instance of Config.
func NewConfig() *Config {
	return &Config{
		JsonnetLibPaths: make([]string, 0),

		dispatchers: map[string]*Dispatcher{},
	}
}

// Watch will call `fn`` when key `k` is updated. It returns a
// cancel function.
func (c *Config) Watch(k string, fn func(interface{})) func() {
	d := c.dispatcher(k)
	return d.Watch(fn)
}

func (c *Config) dispatcher(k string) *Dispatcher {
	d, ok := c.dispatchers[k]
	if !ok {
		d = NewDispatcher()
		c.dispatchers[k] = d
	}

	return d
}

func (c *Config) dispatch(k string, msg interface{}) {
	d := c.dispatcher(k)
	d.Dispatch(msg)
}

// Update updates the configuration.
func (c *Config) Update(update map[string]interface{}) error {
	for k, v := range update {
		switch k {
		case CfgJsonnetLibPaths:
			paths, err := interfaceToStrings(v)
			if err != nil {
				return errors.Wrapf(err, "setting %q", CfgJsonnetLibPaths)
			}

			c.JsonnetLibPaths = paths
			c.dispatch(CfgJsonnetLibPaths, paths)
		default:
			return errors.Errorf("setting %q is unknown to the jsonnet language server", k)
		}
	}
	return nil
}

func (c *Config) String() string {
	data, err := json.Marshal(c)
	if err != nil {
		panic(fmt.Sprintf("marshaling config to JSON: %v", err))
	}
	return string(data)
}

func interfaceToStrings(v interface{}) ([]string, error) {
	switch v := v.(type) {
	case []interface{}:
		var out []string
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, errors.Errorf("item was not a string")
			}

			out = append(out, str)
		}

		return out, nil
	case []string:
		return v, nil
	default:
		return nil, errors.Errorf("unable to convert %T to array of strings", v)
	}
}