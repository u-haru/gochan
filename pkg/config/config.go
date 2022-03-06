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
	return nil
}

func (c *Config) SetParent(p *Config) {
	c.parent = p
}

func (c *Config) Get(k string, to interface{}) error {
	if c.vals == nil {
		c.vals = map[string]interface{}{}
		return errors.New("no such key")
	}
	v, ok := (c.vals)[k]
	if !ok {
		if c.parent != nil {
			return c.parent.Get(k, to)
		}
		return errors.New("no such key")
	}

	rt := reflect.ValueOf(to)
	if rt.Kind() != reflect.Ptr || rt.IsNil() {
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

func (c *Config) GetInt(k string) (int, error) {
	var i int
	err := c.Get(k, &i)
	if err != nil {
		f, err := c.GetFloat64(k)
		if err != nil {
			return 0, err
		}
		if math.Floor(f) == f {
			return int(f), nil
		}
	}
	return i, nil
}

func (c *Config) GetFloat64(k string) (float64, error) {
	var i float64
	err := c.Get(k, &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func (c *Config) GetString(k string) (string, error) {
	var i string
	err := c.Get(k, &i)
	if err != nil {
		return "", err
	}
	return i, nil
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
