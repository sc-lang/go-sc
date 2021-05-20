// Copyright (c) 2021 the SC authors. All rights reserved. MIT License.

package sc_test

import (
	"fmt"

	"github.com/sc-lang/go-sc"
	"github.com/sc-lang/go-sc/scparse"
)

func ExampleUnmarshal() {
	scData := []byte(`{
  name: "foo"
  memory: 256
  required: true
}`)
	type Config struct {
		Name       string `sc:"name"`
		Memory     int    `sc:"memory"`
		IsRequired bool   `sc:"required"`
	}
	var config Config
	err := sc.Unmarshal(scData, &config)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Printf("%+v\n", config)
	// Output:
	// {Name:foo Memory:256 IsRequired:true}
}

func ExampleUnmarshal_variables() {
	scData := []byte(`{
  code: ${id}
  path: "/home/${user}/data"
}`)
	type Config struct {
		Code int    `sc:"code"`
		Path string `sc:"path"`
	}
	var config Config
	vars := sc.MustVariables(map[string]interface{}{
		"id":   145,
		"user": "ted",
	})
	err := sc.Unmarshal(scData, &config, sc.WithVariables(vars))
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Printf("%+v\n", config)
	// Output:
	// {Code:145 Path:/home/ted/data}
}

func ExampleUnmarshalNode() {
	scData := []byte(`{
  name: "test"
  ports: [
    { src: 8080, dst: 8080 }
	{ src: 80, dst: 80 }
  ]
}`)
	type Config struct {
		Name  string
		Ports scparse.ValueNode
	}
	var config Config
	err := sc.Unmarshal(scData, &config)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	// Can inspect node
	fmt.Printf("%v\n", config.Ports.Type())

	// Finish the unmarshaling process
	type Port struct {
		Src int
		Dst int
	}
	var ports []Port
	err = sc.UnmarshalNode(config.Ports, &ports)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Printf("%+v\n", ports)

	// Output:
	// List
	// [{Src:8080 Dst:8080} {Src:80 Dst:80}]
}

func ExampleMarshal() {
	type Config struct {
		Name       string `sc:"name"`
		Memory     int    `sc:"memory"`
		IsRequired bool   `sc:"required"`
	}
	config := Config{
		Name:       "foo",
		Memory:     256,
		IsRequired: true,
	}
	b, err := sc.Marshal(config)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	fmt.Printf("%s\n", b)

	// Output:
	// {
	//   name: "foo"
	//   memory: 256
	//   required: true
	// }
}
