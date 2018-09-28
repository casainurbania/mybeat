// +build windows

package reloader

// use named pipe for ipc

import (
	"bufio"
	"fmt"

	"github.com/elastic/beats/libbeat/cfgfile"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/natefinch/npipe"
)

const namedPipe = "_win_ipc_pipe"
const reloadMsg = "bkreload"
const reloadRepMsg = "bkreload\n"

// NewReloader creates new Reloader instance for the given config
func NewReloader(name string, hadnler ReloadIf) *Reloader {
	return &Reloader{
		hadnler: hadnler,
		name:    name,
		done:    make(chan struct{}),
	}
}

// Run runs the reloader
func (rl *Reloader) Run(path string) error {
	logp.Info("Config reloader started")
	// listen
	ln, err := npipe.Listen(`\\.\pipe\` + rl.name + namedPipe)
	if err != nil {
		return err
	}

	go rl.signalHandler(ln)
	return nil
}

// Stop stops the reloader and waits for all modules to properly stop
func (rl *Reloader) Stop() {
	close(rl.done)
}

func (rl *Reloader) signalHandler(ln *npipe.PipeListener) {
	for {
		conn, err := ln.Accept()
		if err == npipe.ErrClosed {
			logp.Info("config reloader stopped")
			return
		}
		if err != nil {
			// handle error
			logp.Err("Error accepting connection: %v", err)
			continue
		}

		// handle connection like any other net.Conn
		r := bufio.NewReader(conn)
		msg, err := r.ReadString('\n')
		if err != nil {
			logp.Err("Error reading from server connection: %v", err)
			continue
		}
		if msg != reloadRepMsg {
			logp.Err("Read incorrect message. Expected '%s', got '%s'", reloadMsg, msg)
			continue
		}
		logp.Info("reloader recv msg=%s", msg)

		// close client
		if err := conn.Close(); err != nil {
			logp.Err("Error closing server side of connection: %v", err)
			continue
		}

		logp.Info("reloading...")

		// get new config
		c, err := cfgfile.Load("")
		if err != nil {
			logp.Err("Error loading config: %s", err)
			continue
		}
		logp.Info("reloader get config:%+v", c)

		rl.hadnler.Reload(c)
	}
}

// ReloadEvent send reload event
func ReloadEvent(name, path string) error {
	fmt.Println("sending reload msg")

	// Caution: this is not normall path
	conn, err := npipe.Dial(`\\.\pipe\` + name + namedPipe)
	if err != nil {
		return err
	}
	defer conn.Close()

	// send msg
	if _, err := fmt.Fprintln(conn, reloadMsg); err != nil {
		return err
	}

	return nil
}
