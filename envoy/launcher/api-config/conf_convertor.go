package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type AuthConf struct {
	Policy     string `yaml:"policy"`
	Permission string `yaml:"permission"`
}

const (
	// auth policies
	apRequired = "required"
	apOptional = "optional"
	apNoNeed   = "no-need"
)

func (c *AuthConf) Validate() error {
	switch c.Policy {
	case apRequired, apOptional, apNoNeed:
		return nil
	default:
		return fmt.Errorf("unknown auth policy %s", c.Policy)
	}
}

type ClusterConf struct {
	Name string `yaml:"name"`
	Addr string `yaml:"addr"`
}

func (c ClusterConf) Validate() error {
	parts := strings.Split(c.Addr, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid address %s", c.Addr)
	}
	_, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid port number %s", parts[1])
	}

	return nil
}

func (c ClusterConf) AddrHost() string {
	return strings.Split(c.Addr, ":")[0]
}

func (c ClusterConf) AddrPort() string {
	return strings.Split(c.Addr, ":")[1]
}

type APIConf struct {
	APIsDescr []struct {
		Name    string    `yaml:"name"`
		Cluster string    `yaml:"cluster"`
		Auth    *AuthConf `yaml:"auth"`
		Methods []struct {
			Name string    `yaml:"name"`
			Auth *AuthConf `yaml:"auth"`
		} `yaml:"methods"`
	} `yaml:"apis"`

	Clusters []ClusterConf `yaml:"clusters"`
	APIRoute string        `yaml:"api_route"`
}

func (c *APIConf) Validate() error {
	clusters := make(map[string]string)
	apis := make(map[string]string)
	methods := make(map[string]bool)

	if len(c.APIRoute) == 0 || c.APIRoute[0] != '/' {
		return fmt.Errorf("invalid api_route")
	}

	for _, cl := range c.Clusters {
		if _, ok := clusters[cl.Name]; ok {
			return fmt.Errorf("cluster %s is defined twice", cl.Name)
		}
		clusters[cl.Name] = cl.Addr
		if err := cl.Validate(); err != nil {
			return fmt.Errorf("invalid cluster %s definition: %s", cl.Name, err)
		}
	}

	for _, api := range c.APIsDescr {
		if _, ok := apis[api.Name]; ok {
			return fmt.Errorf("API %s is defined twice", api.Name)
		}
		if _, ok := clusters[api.Cluster]; !ok {
			return fmt.Errorf("cluster %s for API %s is not defined", api.Cluster, api.Name)
		}
		apis[api.Name] = api.Cluster

		if api.Auth != nil {
			if err := api.Auth.Validate(); err != nil {
				return err
			}
		}

		for _, m := range api.Methods {
			fullMethod := fmt.Sprintf("%s/%s", api.Name, m.Name)
			if _, ok := methods[fullMethod]; ok {
				return fmt.Errorf("method %s is already defined", fullMethod)
			}
			methods[fullMethod] = true

			if m.Auth != nil {
				if err := m.Auth.Validate(); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func LoadConfig(file string) (*APIConf, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	c := &APIConf{}
	err = yaml.Unmarshal(data, c)
	if err != nil {
		return nil, err
	}

	return c, nil
}
