package beater

import (
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"
	"github.com/mybeat/config"
)

type Mybeat struct {
	timer  *time.Timer
	config config.Config
	client publisher.Client
	done   chan bool
}

type Beater interface {
	// The main event loop. This method should block until signalled to stop by an
	// invocation of the Stop() method.
	Run(b *beat.Beat) error
	// Stop is invoked to signal that the Run method should finish its execution.
	// It will be invoked at most once.
	Stop()
	// Reload will send new config to user after run 'reload' command
	// add by bkdata
	Reload(*common.Config)
}

// Creates beater
func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	config := config.DefaultConfig
	if err := cfg.Unpack(&config); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	// TODO parse config here
	bt := &Mybeat{
		done:   make(chan bool),
		config: config,
	}
	return bt, nil
}

func (bt *Mybeat) Run(b *beat.Beat) error {
	logp.Info("mybeat is running! Hit CTRL-C to stop it.")

	bt.client = b.Publisher.Connect()

	// TODO add code here
	bt.timer = time.NewTimer(bt.config.Period)
	counter := 1
	for {
		select {
		case <-bt.done:
			return nil
		case <-bt.timer.C:
			bt.timer.Reset(bt.config.Period)
		}
		
		event := common.MapStr{
			"timestamp": common.Time(time.Now()),
			"dataid":    bt.config.DataID, // must have dataid field
			"type":      b.Name,
			"counter":   counter,
			"sys": 
		}

		// TODO event.update([追加采集字段]])
		uptime() //print sys performence on AIX

		bt.client.PublishEvent(event)
		logp.Info("Event sent")
		counter++
	}
}

func (bt *Mybeat) Stop() {
	logp.Info("shutting down.")
	bt.client.Close()
	close(bt.done)
}

func (bt *Mybeat) Reload(cfg *common.Config) {
	logp.Info("reload")
	config := config.DefaultConfig
	err := cfg.Unpack(&config)
	if err != nil {
		logp.Err("error reading configuration file")
	}
	logp.Info("config:%+v", config)
	// TODO parse config here
	bt.config = config
}

const (
	moduleName    = "mymodule"
	metricSetName = "mymetricset"
	host          = "localhost"
	elapsed       = time.Duration(500 * time.Millisecond)
	tag           = "alpha"
)

var (
	startTime = time.Now()
	errFetch  = errors.New("error fetching data")
	tags      = []string{tag}
)
//使用样例
func uptime() {
	cmd := exec.Command("uptime")
	buf, _ := cmd.Output()
	return buf
}
