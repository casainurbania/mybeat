package monitor

import (
	"os"
	"time"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/shirou/gopsutil/process"
)

// MonitorSender
type MonitorSender interface {
	// event must contain 'dataid' field
	Report(dataid int32, event interface{}) error
}

// ResourceCollector
type ResourceCollector struct {
	dataid   int32
	period   time.Duration
	sender   MonitorSender
	quitChan chan bool
}

// NewResourceCollector
func NewResourceCollector(dataid int32, sender MonitorSender) ResourceCollector {
	c := ResourceCollector{
		sender:   sender,
		quitChan: make(chan bool),
		dataid:   dataid,
		period:   time.Minute,
	}
	return c
}

// Start collector
func (c *ResourceCollector) Start() {
	logp.Info("start resource collector, report to %d", c.dataid)
	if c.dataid <= 0 {
		logp.Info("bkmonitor resource collector will do nothing")
		return
	}
	go c.statistic()
}

// Stop collector
func (c *ResourceCollector) Stop() {
	logp.Info("bkmonitor resource collector stop")
	close(c.quitChan)
}

// statistic : report process resouce and report
func (c *ResourceCollector) statistic() {
	pid := os.Getpid()
	ticker := time.NewTicker(c.period)
	for {
		select {
		case <-ticker.C:
			// collect cpu, mem, fd
			p, err := process.NewProcess(int32(pid))
			if err != nil {
				logp.Err("bkmonitor resource collector get pid err, %v", err)
				break
			}

			// TODO
			cpu, err := p.Percent(3 * time.Second)
			if err != nil {
				logp.Err("bkmonitor resource collector cpu err, %v", err)
				break
			}

			mem, err := p.MemoryInfo()
			if err != nil {
				logp.Err("bkmonitor resource collector mem err, %v", err)
				break
			}

			fd, err := p.NumFDs()
			// TODO windows not implement now, will return error
			// if err != nil {
			// 	logp.Err("bkmonitor resource collector fd err, %v", err)
			// 	break
			// }

			// report op data
			event := common.MapStr{
				"cpu": cpu,
				"mem": mem,
				"fd":  fd,
			}
			// udp msg can not large then 64K, split to small pkgs
			c.sender.Report(c.dataid, event)
		case <-c.quitChan:
			return
		}
	}
}
