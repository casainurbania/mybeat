package gse

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/elastic/beats/bkdatalib/gselib"
	"github.com/elastic/beats/bkdatalib/monitor"
	bkstorage "github.com/elastic/beats/bkdatalib/storage"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/common/op"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/outputs"
)

const maxSyncAgentInfoTimeout = 10 // unit: second

func init() {
	outputs.RegisterOutputPlugin("gse", New)
}

// Output : gse output, for libbeat output
type Output struct {
	cli        *gselib.GseClient
	tagMonitor *monitor.DataQualityMonitor
	resMonitor *monitor.ResourceCollector
}

// New create a gse client
func New(beatName string, cfg *common.Config, topologyExpire int) (outputs.Outputer, error) {
	c := defaultConfig
	err := cfg.Unpack(&c)
	if err != nil {
		logp.Err("unpack config error, %v", err)
		return nil, err
	}
	logp.Info("gse config: %+v", c)

	// create gse client
	cli, err := gselib.NewGseClient(cfg)
	if err != nil {
		return nil, err
	}
	output := &Output{
		cli: cli,
	}

	// start gse client
	err = output.cli.Start()
	if err != nil {
		logp.Err("init output failed, %v", err)
		return nil, err
	}
	logp.Info("start gse output")

	// wait to get agent info
	agentInfo, err := output.cli.GetAgentInfo()
	count := maxSyncAgentInfoTimeout
	for {
		if count <= 0 {
			return nil, fmt.Errorf("get agent info timeout")
		}
		if agentInfo.IP != "" {
			break
		}
		count--
		// sleep 1s, then continue to get agent info
		time.Sleep(1 * time.Second)
		agentInfo, err = output.cli.GetAgentInfo()
	}

	// start data quality monitor
	info := monitor.DataQualityMonitorInfo{
		Module:        "collector",
		Component:     "collector",
		UniqType:      beatName,
		IP:            agentInfo.IP,
		CloudID:       int(agentInfo.Cloudid),
		CompanyID:     int(agentInfo.Bizid),
		DownModule:    "collector",
		DownComponent: "agent",
	}
	m := monitor.NewDataQualityMonitor(info, output)
	m.DataID = c.MonitorID
	m.Start()
	logp.Info("start data monitor, report to %d", c.MonitorID)
	output.tagMonitor = &m

	if c.ResourceID > 0 {
		// start resource monitor
		resMonitor := monitor.NewResourceCollector(c.ResourceID, output)
		resMonitor.Start()
		logp.Info("start resource monitor, report to %d", c.ResourceID)
		output.resMonitor = &resMonitor
	}

	return output, nil
}

// PublishEvent implement output interface
// data is event, must contain 'dataid' filed
// data will attach agent info, see publishEventAttachInfo
func (c *Output) PublishEvent(sig op.Signaler, opts outputs.Options, data outputs.Data) error {
	// get dataid from event
	val, err := data.Event.GetValue("dataid")
	if err != nil {
		logp.Err("event lost dataid field, %v", err)
		return err
	}

	dataid := c.getdataid(val)
	if dataid <= 0 {
		return fmt.Errorf("dataid %d <= 0", dataid)
	}

	if err := c.publishEventAttachInfo(dataid, data.Event); err != nil {
		return err
	}

	op.SigCompleted(sig)
	return nil
}

// Close : close gse out put
func (c *Output) Close() error {
	logp.Err("gse output close")
	if c.tagMonitor != nil {
		c.tagMonitor.Stop()
	}
	if c.resMonitor != nil {
		c.resMonitor.Stop()
	}
	c.cli.Close()
	return nil
}

// publishEventAttachInfo attach agentinfo and gseindex
// will add bizid, cloudid, ip, gseindex
func (c *Output) publishEventAttachInfo(dataid int32, data common.MapStr) error {
	// add gseindex
	if ok, _ := data.HasKey("gseindex"); !ok {
		index := uint64(0)
		if indexStr, err := bkstorage.Get("gseindex"); nil == err {
			if index, err = strconv.ParseUint(indexStr, 10, 64); nil != err {
				logp.Err("fail to get gseindex %v", err)
				index = 0
			}
		}
		index += 1
		bkstorage.Set("gseindex", fmt.Sprintf("%v", index), 0)
		data.Put("gseindex", index)
	}

	// add bizid, cloudid, ip
	info, _ := c.cli.GetAgentInfo()
	if len(info.IP) == 0 {
		return fmt.Errorf("agent info is empty")
	}
	data["bizid"] = info.Bizid
	data["cloudid"] = info.Cloudid
	data["ip"] = info.IP

	logp.Debug("gse", "gse event: %d, %v", dataid, data)

	// if is op data, send with op protocol
	if ok, _ := data.HasKey("_opdata"); ok {
		data.Delete("_opdata")
		logp.Debug("gse", "gse op event: %d, %v", dataid, data)
		return c.reportOpData(dataid, data)
	} else {
		return c.reportCommonData(dataid, data)
	}
}

// Report implement interface for resource
// send op data
func (c *Output) Report(dataid int32, data interface{}) error {
	if dataid <= 0 {
		return fmt.Errorf("dataid %d <= 0", dataid)
	}

	logp.Debug("gse", "report data to %d", dataid)
	// report op data
	event := common.MapStr{
		"_opdata": true,
		"data":    data,
		"dataid":  dataid,
	}
	return c.publishEventAttachInfo(dataid, event)
}

// ReportRaw implement interface for monitor
// send op raw data, without attach anything
func (c *Output) ReportRaw(dataid int32, data interface{}) error {
	if dataid <= 0 {
		return fmt.Errorf("dataid %d <= 0", dataid)
	}

	buf, err := json.Marshal(data)
	if err != nil {
		logp.Err("convert to json faild: %v", err)
		return err
	}

	logp.Debug("gse", "report data to %d", dataid)
	// report op data

	msg := gselib.NewGseOpMsg(buf, dataid, 0, 0, 0)

	// TODO compatible op data bug fixed after agent D48
	// send every op data with new connection
	c.cli.SendWithNewConnection(msg)
	//c.cli.Send(msg)

	return nil
}

// reportCommonData send common data
func (c *Output) reportCommonData(dataid int32, data common.MapStr) error {
	// change data to json format
	buf, err := json.Marshal(data)
	if err != nil {
		logp.Err("json marshal failed, %v", err)
		return err
	}

	// new dynamic msg
	msg := gselib.NewGseDynamicMsg(buf, dataid, 0, 0)
	tag := c.tagMonitor.GetTag()
	msg.AddMeta(monitor.BKMonitorTag, tag)

	// send data
	c.cli.Send(msg)

	// monitor tag
	c.tagMonitor.OutputInc(int(dataid), tag)

	return nil
}

// reportOpData send op data
func (c *Output) reportOpData(dataid int32, data common.MapStr) error {
	buf, err := json.Marshal(data)
	if err != nil {
		logp.Err("convert to json faild: %v", err)
		return err
	}

	msg := gselib.NewGseOpMsg(buf, dataid, 0, 0, 0)

	// TODO compatible op data bug fixed after agent D48
	// send every op data with new connection
	c.cli.SendWithNewConnection(msg)
	//c.cli.Send(msg)

	return nil
}

func (c *Output) getdataid(dataID interface{}) int32 {
	switch dataID.(type) {
	case int, int8, int16, int32, int64:
		return int32(reflect.ValueOf(dataID).Int())
	case string:
		dataid, err := strconv.ParseInt(dataID.(string), 10, 32)
		if err != nil {
			logp.Err("can not get dataid, %s", dataID.(string))
			return -1
		}
		return int32(dataid)
	default:
		logp.Err("unexpected type %T for the dataid ", dataID)
		return 0
	}

}
