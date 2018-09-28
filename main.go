package main

import (
	"os"

	"github.com/elastic/beats/libbeat/beat"

	"github.com/mybeat/beater"
)

func main() {
	err := beat.Run("mybeat", "", beater.New)
	if err != nil {
		os.Exit(1)
	}
}
