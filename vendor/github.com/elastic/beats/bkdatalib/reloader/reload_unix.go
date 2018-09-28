// +build linux darwin aix

package reloader

// use signal SIGUSR1 for ipc

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/elastic/beats/libbeat/cfgfile"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/pidfile"
)

// NewReloader creates new Reloader instance for the given config
func NewReloader(name string, hadnler ReloadIf) *Reloader {
	return &Reloader{
		hadnler: hadnler,
		name:    name,
		done:    make(chan struct{}),
	}
}

// Run runs the reloader
func (rl *Reloader) Run(_ string) error {
	logp.Info("Config reloader started")

	// watch SIGUSR1
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go rl.signalHandler(c)
	return nil
}

// Stop stops the reloader and waits for all modules to properly stop
func (rl *Reloader) Stop() {
	close(rl.done)
}

func (rl *Reloader) signalHandler(c chan os.Signal) {
	for {
		select {
		case <-rl.done:
			logp.Info("config reloader stopped")
			return
		case s := <-c:
			logp.Info("got signal: %+v", s)
			if s == syscall.SIGUSR1 { // reload signal
				logp.Info("reloading...")

				// get new config
				c, err := cfgfile.Load("")
				if err != nil {
					logp.Err("Error loading config: %s", err)
					continue
				}
				c, err = c.Child(rl.name, -1)
				if err != nil {
					logp.Err("Error loading config: %s", err)
					continue
				}

				logp.Info("reloader get config:%+v", c)

				rl.hadnler.Reload(c)
			}
		}
	}
}

// ReloadEvent send reload event
func ReloadEvent(_, pidFilePath string) error {
	fmt.Println("sending reload signal")
	// get pid from pidfile

	pid, err := pidfile.GetPid(pidFilePath)
	if err != nil {
		return err
	}

	// send signal
	proc, err := os.FindProcess(pid)
	if err != nil {
	}
	proc.Signal(syscall.SIGUSR1)
	return nil
}
