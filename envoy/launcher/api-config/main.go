package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	apiConfPath      string
	envoyConfOutPath string
)

func init() {
	flag.StringVar(&apiConfPath, "api-conf", "config.yaml", "API config file path")
	flag.StringVar(&envoyConfOutPath, "out-envoy-conf", "conf_out.yaml", "out Envoy config file")
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[FATAL] %s\n", r)
			os.Exit(1)
		}
	}()
	flag.Parse()

	c, err := LoadConfig(apiConfPath)
	if err != nil {
		panic(err)
	}

	if err := c.Validate(); err != nil {
		panic(err)
	}

	err = GenerateEnvoyConfig(c, envoyConfOutPath)
	if err != nil {
		panic(err)
	}

	fmt.Println("done")
}
