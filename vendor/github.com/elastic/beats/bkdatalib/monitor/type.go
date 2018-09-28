package monitor

/*
精简版本，省略了一些不需要上报的字段
*/

// Summary :
type Summary struct {
	Time     int64    `json:"time"`
	Info     Info     `json:"info"`
	Location Location `json:"location"`
	Metrics  Metric   `json:"metrics"`
}

// Info :
type Info struct {
	Module      string            `json:"module"`
	Component   string            `json:"component"`
	PhysicalTag Tag               `json:"physical_tag"`
	LogicalTag  Tag               `json:"logical_tag"`
	CustomTag   map[string]string `json:"custom_tags"`
}

type Tag struct {
	Tag  string            `json:"tag"`
	Desc map[string]string `json:"desc"`
}

type Location struct {
	// upstream   []Stream `json:"upstream"`
	Downstream []Stream `json:"downstream"`
}

type Stream struct {
	Module    string `json:"module"`
	Component string `json:"component"`
	Logical   Tag    `json:"logical_tag"`
}

type Metric struct {
	Data DataMetric `json:"data_monitor"`
	// resource map[string]string `json:"resource_monitor"`
	// custom   map[string]string `json:"custom_metrics"`
}

type DataMetric struct {
	Loss DataLoss `json:"data_loss"`
	// delay DataDelay `json:"data_delay"`
}

type DataLoss struct {
	Input  *Count                `json:"input"`
	Output *Count                `json:"output"`
	Drop   map[string]*DropCount `json:"data_drop"`
}

func NewDataLoss() DataLoss {
	c := DataLoss{}
	c.Input = &Count{}
	c.Output = &Count{}
	c.Input.Tags = make(map[string]uint64)
	c.Output.Tags = make(map[string]uint64)
	c.Drop = make(map[string]*DropCount)
	return c
}

type Count struct {
	Tags      map[string]uint64 `json:"tags"`
	Sum       uint64            `json:"total_cnt"`
	Increment uint64            `json:"total_cnt_increment"`
}

func (c *Count) Inc(tag string) {
	c.Sum++
	c.Increment++
	if _, ok := c.Tags[tag]; ok {
		c.Tags[tag]++
	} else {
		c.Tags[tag] = 1
	}
}

type DropCount struct {
	Count  uint64 `json:"cnt"`
	Reason string `json:"reason"`
}

func (c *DropCount) Inc() {
	c.Count++
}

type Item struct {
	dataid int
	tag    string
}
