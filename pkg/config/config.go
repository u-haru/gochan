package config

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
)

type Config struct {
	vals   map[string]interface{}
	parent *Config
}

var (
	errNoKey        = errors.New("no such key")
	errNotPointer   = errors.New("it's not pointer")
	errTypeMismatch = errors.New("type mismatch")
	errNotSettable  = errors.New("it cannot be set")
)

func (c *Config) LoadJson(from io.Reader) error {
	js, err := io.ReadAll(from)
	if err != nil {
		return err
	}

	err = json.Unmarshal(js, &c.vals)
	if err != nil {
		return err
	}
	for i, v := range c.vals {
		rv := reflect.TypeOf(v)
		if rv.Kind() == reflect.Float64 || rv.Kind() == reflect.Float32 {
			f, ok := v.(float64)
			if ok && math.Floor(f) == f {
				c.vals[i] = int(f)
			}
		}
	}
	return nil
}

func (c *Config) SetParent(p *Config) {
	c.parent = p
}

func (c *Config) Get(k string, to interface{}) error {
	if c.vals == nil {
		c.vals = map[string]interface{}{}
	}
	v, ok := (c.vals)[k]
	if !ok {
		if c.parent != nil {
			return c.parent.Get(k, to)
		}
		return errNoKey
	}

	rt := reflect.ValueOf(to)
	if rt.IsNil() || rt.Kind() != reflect.Ptr {
		return errNotPointer
	}
	p := rt.Elem()
	rv := reflect.TypeOf(v)
	if t := p.Type(); t != rv && t.Kind() != reflect.Interface {
		return errTypeMismatch
	}
	if !p.CanSet() {
		return errNotSettable
	}

	p.Set(reflect.ValueOf(v))
	return nil
}

func (c *Config) GetRaw(k string) (interface{}, error) {
	v, ok := c.vals[k]
	if !ok {
		if c.parent != nil {
			return c.parent.GetRaw(k)
		}
		return nil, errNoKey
	}
	return v, nil
}

func (c *Config) GetInt(k string) (int, error) {
	v, err := c.GetRaw(k)
	if err != nil {
		return 0, err
	}
	i, ok := v.(int)
	if !ok {
		return 0, errTypeMismatch
	}
	return i, nil
}

func (c *Config) GetFloat64(k string) (float64, error) {
	v, err := c.GetRaw(k)
	if err != nil {
		return 0, err
	}
	f, ok := v.(float64)
	if !ok {
		return 0, errTypeMismatch
	}
	return f, nil
}

func (c *Config) GetString(k string) (string, error) {
	v, err := c.GetRaw(k)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", errTypeMismatch
	}
	return s, nil
}

func (c *Config) GetBool(k string) (bool, error) {
	v, err := c.GetRaw(k)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, errTypeMismatch
	}
	return b, nil
}

func (c *Config) Set(k string, from interface{}) error {
	if c.vals == nil {
		c.vals = map[string]interface{}{}
	}
	c.vals[k] = from
	return nil
}

func (c *Config) ExportJson(to io.Writer) error {
	b, err := json.Marshal(c.vals)
	if err != nil {
		return err
	}

	to.Write(b)
	return nil
}
