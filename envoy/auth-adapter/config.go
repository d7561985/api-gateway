package main

import (
	"fmt"
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	// auth policies
	apRequired = "required"
	apOptional = "optional"
	apNoNeed   = "no-need"
)

type rateLimitConf struct {
	Period time.Duration `yaml:"period"`
	Count  int           `yaml:"count"`
	Delay  time.Duration `yaml:"delay"`
}

type authConf struct {
	Policy     string         `yaml:"policy"`
	Permission string         `yaml:"permission"`
	ReCaptcha  bool           `yaml:"need_recaptcha"`
	RateLimit  *rateLimitConf `yaml:"rate_limit"`
}

func (c authConf) NoNeed() bool {
	return c.Policy == apNoNeed
}

func (c authConf) Optional() bool {
	return c.Policy == apOptional
}

func (c authConf) Required() bool {
	return c.Policy == apRequired
}

func (c authConf) NeedReCaptcha() bool {
	return c.ReCaptcha
}

func (c authConf) Valid() bool {
	switch c.Policy {
	case apRequired, apOptional, apNoNeed:
		return true
	default:
		return false
	}
}

type APIConf struct {
	APIsDescr []struct {
		Name    string    `yaml:"name"`
		Auth    *authConf `yaml:"auth"`
		Methods []struct {
			Name string    `yaml:"name"`
			Auth *authConf `yaml:"auth"`
		} `yaml:"methods"`
	} `yaml:"apis"`

	methodsIndex map[string]*authConf
}

func LoadConfig(file string) (*APIConf, error) {
	c := &APIConf{}

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, c)
	if err != nil {
		return nil, err
	}

	mi := make(map[string]*authConf)
	for _, api := range c.APIsDescr {
		if api.Auth != nil {
			if !api.Auth.Valid() {
				return nil, fmt.Errorf("unknown auth policy %s for service %s", api.Auth.Policy, api.Name)
			}

			mi[api.Name] = api.Auth
		}

		for _, method := range api.Methods {
			fullPath := fmt.Sprintf("%s/%s", api.Name, method.Name)
			if _, ok := mi[fullPath]; ok {
				return nil, fmt.Errorf("method %s is already configured", fullPath)
			}

			if method.Auth != nil {
				if !method.Auth.Valid() {
					return nil, fmt.Errorf("unknown auth policy %s for method %s", method.Auth.Policy, fullPath)
				}
				mi[fullPath] = method.Auth

			}
		}
	}

	c.methodsIndex = mi

	return c, nil
}

func (c *APIConf) GetRequestedPermissions(service, method string) *authConf {
	if auth, ok := c.methodsIndex[method]; ok {
		return auth
	}

	return c.methodsIndex[service]
}
