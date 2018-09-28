package monitor

import (
	"strconv"
	"time"

	bkcommon "github.com/elastic/beats/bkdatalib/common"
	"github.com/elastic/beats/libbeat/logp"
)

/*
埋点数据，提供数据质量监控使用
每分钟上报埋点信息，如果没有数据转发，则不上报
*/

// BKMonitorTag : used for monitor
const BKMonitorTag = "tag"

// DataQualityMonitorSender :
// dataid is default monitor dataid, pass to caller
// data is summary data
type DataQualityMonitorSender interface {
	ReportRaw(dataid int32, data interface{}) error
}

// DataQualityMonitorInfo : monitor basic info
type DataQualityMonitorInfo struct {
	Module        string
	Component     string
	UniqType      string
	IP            string
	CloudID       int
	CompanyID     int
	DownModule    string
	DownComponent string
	ComplexID     int // generate by program
}

// DataQualityMonitor :
type DataQualityMonitor struct {
	DataID     int32
	basic      DataQualityMonitorInfo
	prefixTag  string
	sender     DataQualityMonitorSender
	quitChan   chan bool
	info       Info
	currentTag string
	current    map[int]DataLoss // current statistic data
	output     chan Item
	drop       chan Item
}

// NewDataQualityMonitor : new DataQualityMonitor
func NewDataQualityMonitor(info DataQualityMonitorInfo, sender DataQualityMonitorSender) DataQualityMonitor {
	c := DataQualityMonitor{
		basic:  info,
		sender: sender,
	}

	// complex id contains cloudid and companyid, produced by gse
	// complexID = (cloudID << 22) | (companyID & 0x003fffff)
	c.basic.ComplexID = (info.CloudID << 22) | (info.CompanyID & 0x003fffff)

	c.prefixTag = info.Component + "|" +
		info.UniqType + "|" +
		strconv.Itoa(info.ComplexID) + "|" +
		info.IP
	c.info.Module = info.Module
	c.info.Component = info.Component

	// component|type|complexid|ip
	c.info.PhysicalTag.Tag = c.prefixTag
	c.info.PhysicalTag.Desc = make(map[string]string)
	// c.info.PhysicalTag.Desc["module"] = info.module
	c.info.PhysicalTag.Desc["component"] = info.Component
	c.info.PhysicalTag.Desc["type"] = info.UniqType
	c.info.PhysicalTag.Desc["complexid"] = strconv.Itoa(info.ComplexID)
	c.info.PhysicalTag.Desc["ip"] = info.IP

	c.current = make(map[int]DataLoss)
	c.output = make(chan Item)
	c.drop = make(chan Item)
	return c
}

// Start : start.
func (c *DataQualityMonitor) Start() {
	logp.Info("bkmonitor start")
	c.quitChan = make(chan bool)
	c.updateTag()
	go c.statistic()
}

// Stop : stop
func (c *DataQualityMonitor) Stop() {
	logp.Err("bkmonitor stop")
	close(c.quitChan)
}

// SetCustomTag : add custom tag
func (c *DataQualityMonitor) SetCustomTag(key, value string) {
	if c.info.CustomTag == nil {
		c.info.CustomTag = make(map[string]string)
	}
	c.info.CustomTag["key"] = value
}

// GetTag : get tag
func (c *DataQualityMonitor) GetTag() string {
	return c.currentTag
}

// OutputInc : output increase
// tag is returned by GetTag
func (c *DataQualityMonitor) OutputInc(dataid int, tag string) {
	if dataid > 0 {
		c.output <- Item{dataid: dataid, tag: tag}
		return
	}
}

// DropInc : drop data increase.
// exp. send failed
func (c *DataQualityMonitor) DropInc(dataid int, reason string) {
	if dataid > 0 {
		c.drop <- Item{dataid: dataid, tag: reason}
		return
	}
}

// statistic : do summary every minutes
func (c *DataQualityMonitor) statistic() {
	timer1M := time.NewTimer(1 * time.Minute)
	for {
		select {
		case e := <-c.output:
			if _, ok := c.current[e.dataid]; !ok {
				c.current[e.dataid] = NewDataLoss()
			}
			c.current[e.dataid].Output.Inc(e.tag)
		case e := <-c.drop:
			if _, ok := c.current[e.dataid]; !ok {
				c.current[e.dataid] = NewDataLoss()
			}
			if _, ok := c.current[e.dataid].Drop[e.tag]; ok {
				c.current[e.dataid].Drop[e.tag].Inc()
			} else {
				c.current[e.dataid].Drop[e.tag] = &DropCount{
					Count:  1,
					Reason: e.tag,
				}
			}
		case <-timer1M.C:
			timer1M.Reset(1 * time.Minute)
			logp.Debug("bkmonitor", "send summary")
			c.summary()
			c.updateTag()
		case <-c.quitChan:
			logp.Err("bkmonitor statistic quit")
			return
		}
	}
}

// summary : make new summary data, pass to Send()
func (c *DataQualityMonitor) summary() {
	// udp msg can not large then 64K, split to small pkgs
	batchCountMax := 10
	summarys := []Summary{}
	for dataid, count := range c.current {
		// not report dataid if no more data
		if count.Input.Increment == 0 &&
			count.Output.Increment == 0 &&
			len(count.Drop) == 0 {
			continue
		}

		m := Summary{}
		m.Time = time.Now().Unix()

		// info
		m.Info = c.info
		dataIDStr := strconv.Itoa(dataid)
		m.Info.LogicalTag.Tag = dataIDStr
		m.Info.LogicalTag.Desc = make(map[string]string)
		m.Info.LogicalTag.Desc["dataId"] = dataIDStr

		// location --> downstream
		// only has one downstream
		stream := Stream{
			Module:    c.basic.DownModule,
			Component: c.basic.DownComponent,
		}
		stream.Logical = m.Info.LogicalTag
		m.Location.Downstream = append(m.Location.Downstream, stream)

		// metrics
		if count.Input.Increment > 0 {
			m.Metrics.Data.Loss.Input = count.Input
		}
		if count.Output.Increment > 0 {
			m.Metrics.Data.Loss.Output = count.Output
		}
		if len(count.Drop) > 0 {
			m.Metrics.Data.Loss.Drop = count.Drop
		}

		summarys = append(summarys, m)

		// clear cache
		newCount := NewDataLoss()
		newCount.Input.Sum = count.Input.Sum
		newCount.Output.Sum = count.Output.Sum
		c.current[dataid] = newCount
		// send every max count
		if len(summarys) >= batchCountMax {
			c.sender.ReportRaw(c.DataID, summarys)
			summarys = []Summary{}
		}
	}

	// send remain summarys
	if len(summarys) > 0 {
		c.sender.ReportRaw(c.DataID, summarys)
		summarys = []Summary{}
	}
}

// updateTag : update tag with utc timestamp
func (c *DataQualityMonitor) updateTag() {
	c.currentTag = c.prefixTag + "|" + bkcommon.GetUTCTimestamp()
}
