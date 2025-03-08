package main

import (
	"fmt"
	"sort"

	"github.com/spf13/viper"
)

// displays configuration
func displayConfig() {
	allmysettings := viper.AllSettings()
	var keys []string
	for k := range allmysettings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Println("CONFIG:", k, ":", allmysettings[k])
	}
}

// displays help information
func displayHelp() {
	message := `
      --config string       Configuration file: /path/to/file.yaml (default "./config.yaml")
      --displayconfig       Display configuration
      --help                Display help
      --version             Display version
`

	fmt.Println("simple-discord-bot " + applicationVersion)
	fmt.Println(message)
}
