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
		return errors.New("no such key")
	}

	rt := reflect.ValueOf(to)
	if rt.IsNil() || rt.Kind() != reflect.Ptr {
		return errors.New(reflect.TypeOf(to).String() + " isn't pointer")
	}
	p := rt.Elem()
	rv := reflect.TypeOf(v)
	if t := p.Type(); t != rv && t.Kind() != reflect.Interface {
		return errors.New(rt.String() + " isn't type " + rv.String())
	}
	if !p.CanSet() {
		return errors.New(rt.String() + " cannot be set")
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
		return nil, errors.New("no such key")
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
		return 0, errors.New("type mismatch")
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
		return 0, errors.New("type mismatch")
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
		return "", errors.New("type mismatch")
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
		return false, errors.New("type mismatch")
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
