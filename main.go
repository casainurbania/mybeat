package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/mybeat/beater"
)

func main() {
	err := beat.Run("mybeat", "", beater.New)
	if err != nil {
		os.Exit(1)
	}
	cmd := exec.Command("uptime")
	buf, err := cmd.Output()
	fmt.Println(buf)
	fmt.Printf("err: %v", err)

}
