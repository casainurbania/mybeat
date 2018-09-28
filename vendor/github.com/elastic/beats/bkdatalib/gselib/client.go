package gselib

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"time"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

// Config GseClient config
type Config struct {
	RetryTimes    uint          `config:"retrytimes"`
	RetryInterval time.Duration `config:"retryinterval"`
	MsgQueueSize  uint          `config:"mqsize"`
	WriteTimeout  time.Duration `config:"writetimeout"`
	Endpoint      string        `config:"endpoint"`
	//EventBufferMax int32         `config:"eventbuffermax"`
	Nonblock bool `config:"nonblock"` // TODO not used now
}

var defaultConfig = Config{
	MsgQueueSize:  1,
	WriteTimeout:  5 * time.Second,
	Nonblock:      false,
	RetryTimes:    3,
	RetryInterval: 3 * time.Second,
}

// GseClient : gse client
// used for send data and get agent info
type GseClient struct {
	socket       GseConnection
	agentInfo    AgentInfo
	quitChan     chan bool
	msgChan      chan GseMsg // msg queue
	msgQueueSize uint        // msg queue szie
	cfg          Config
}

// NewGseClient create a gse client
// host set to default gse ipc path, different from linux and windows
func NewGseClient(cfg *common.Config) (*GseClient, error) {
	// parse config
	c := defaultConfig
	err := cfg.Unpack(&c)
	if err != nil {
		logp.Err("unpack config error, %v", err)
		return nil, err
	}
	logp.Info("gse client config: %+v", c)

	cli := GseClient{
		cfg:          c,
		msgQueueSize: c.MsgQueueSize,
	}
	cli.socket = NewGseConnection()
	cli.socket.SetWriteTimeout(c.WriteTimeout)
	if c.Endpoint != "" {
		cli.socket.SetHost(c.Endpoint)
	}
	return &cli, nil
}

// Start : start client
// start to recv msg and get agent info
// run as goroutine
func (c *GseClient) Start() error {
	c.msgChan = make(chan GseMsg, c.msgQueueSize)
	c.quitChan = make(chan bool)

	err := c.connect()
	if err != nil {
		return err
	}

	go c.recvMsgFromAgent()
	// default request agent info evry 31s
	go c.updateAgentInfo(time.Second * 31)
	go c.msgSender()
	logp.Info("gse client start")
	return nil
}

// Close : release resources
func (c *GseClient) Close() {
	logp.Err("gse client closed")
	close(c.quitChan)
	c.socket.Close()
	return
}

// ==========================================

// GetAgentInfo : get agent info
// client will update info from gse agent every 1min
// request from agent first time when client start
func (c *GseClient) GetAgentInfo() (AgentInfo, error) {
	return c.agentInfo, nil
}

// Send : send msg to client
// will bolck when queue is full
func (c *GseClient) Send(msg GseMsg) error {
	c.msgChan <- msg
	return nil
}

// SendWithNewConnection : send msg to client with new connection every time
func (c *GseClient) SendWithNewConnection(msg GseMsg) error {
	// new connection
	socket := NewGseConnection()
	err := socket.Dial()
	if err != nil {
		return err
	}
	defer socket.Close()

	retry := 3
	var n int
	for retry > 0 {
		n, err = socket.Write(msg.ToBytes())
		if err == nil {
			logp.Debug("gse", "send size: %d", n)
			break
		} else {
			logp.Err("gse client sendRawData failed, %v", err)
			c.reconnect()
			time.Sleep(1)
			retry--
		}
	}

	logp.Debug("gse", "send with new conneciton")
	return nil
}

// connect : connect to agent
// try to connect again several times until connected
// program will quit if failed finaly
func (c *GseClient) connect() error {
	retry := c.cfg.RetryTimes
	var err error
	for retry > 0 {
		err = c.socket.Dial()
		if err == nil {
			logp.Info("gse client socket connected")
			return nil
		}
		logp.Err("try %d times", c.cfg.RetryTimes-retry)
		time.Sleep(c.cfg.RetryInterval)
		retry--
	}
	return err
}

// reconnect: reconnect to agent
func (c *GseClient) reconnect() {
	logp.Err("gse client reconnecting...")

	// close quitChan will stop updateAgentInfo and msgSender goroutine
	//close(c.quitChan)
	c.socket.Close()

	err := c.connect()
	if err != nil {
		logp.WTF("connect failed, program quit %v", err)
		return
	}
}

// request agent info every interval time
func (c *GseClient) updateAgentInfo(interval time.Duration) {
	logp.Info("gse client start update agent info")
	err := c.requestAgentInfo()
	if err != nil {
		logp.Err("gse client send sync cfg command failed, %v", err)
	}
	for {
		select {
		case <-time.After(interval):
			logp.Debug("gse", "send sync cfg command")
			err := c.requestAgentInfo()
			if err != nil {
				logp.Err("gse client send sync cfg command failed, error %v", err)
				continue
			}
		case <-c.quitChan:
			logp.Err("gse client updateAgentInfo quit")
			return
		}
	}
}

// msgSender : get msg from queue, send it to agent
func (c *GseClient) msgSender() {
	logp.Info("gse client start send msg")
	for {
		select {
		case msg := <-c.msgChan:
			err := c.sendRawData(msg.ToBytes())
			if err != nil {
				// program quit if send error
				logp.Err("gse client send failed")
			}
		case <-c.quitChan:
			logp.Err("gse client msgSender quit")
			return
		}
	}
}

// sendRawData : send binary data
func (c *GseClient) sendRawData(data []byte) error {
	retry := 3
	var err error
	var n int
	for retry > 0 {
		n, err = c.socket.Write(data)
		if err == nil {
			logp.Debug("gse", "send size: %d", n)
			break
		} else {
			logp.Err("gse client sendRawData failed, %v", err)
			c.reconnect()
			time.Sleep(1)
			retry--
		}
	}
	return err
}

// RequestAgentInfo : request agent info
func (c *GseClient) requestAgentInfo() error {
	logp.Debug("gse", "request agent info")
	msg := NewGseRequestConfMsg()
	return c.Send(msg)
}

// agentInfoMsgHandler: parse to agent info
func (c *GseClient) agentInfoMsgHandler(buf []byte) {
	if err := json.Unmarshal(buf, &c.agentInfo); nil != err {
		logp.Err("gse client data is not json, %s", string(buf))
	}
	logp.Debug("gse", "update agent info, %+v", c.agentInfo)
}

func (c *GseClient) recvMsgFromAgent() {
	logp.Info("gse client start recv msg")
	for {
		// read head
		headbufLen := 8 // GseLocalCommandMsg size
		headbuf := make([]byte, headbufLen)
		len, err := c.socket.Read(headbuf)

		// err handle
		if err == io.EOF {
			// socket closed by agent
			logp.Err("socket closed by remote")
			c.reconnect()
			continue
		} else if err != nil {
			logp.Err("gse client recv err %v", err)
			break
		} else if len != headbufLen {
			logp.Err("gse client recv only %d bytes", len)
			continue
		}

		logp.Debug("gse", "recv len : %d", len)
		//logp.Debug("gse", "headbuf : %s", headbuf)

		// get type and data len
		var msg GseLocalCommandMsg
		msg.MsgType = binary.BigEndian.Uint32(headbuf[:4])
		msg.BodyLen = binary.BigEndian.Uint32(headbuf[4:])
		logp.Debug("gse", "msg type=%d, len=%d", msg.MsgType, msg.BodyLen)

		// TODO now only has GSE_TYPE_GET_CONF type
		if msg.MsgType == GSE_TYPE_GET_CONF {
			// read data
			databuf := make([]byte, msg.BodyLen)
			if _, err := c.socket.Read(databuf); nil != err && err != io.EOF {
				logp.Err("gse client read err, %v", err)
				continue
			}
			c.agentInfoMsgHandler(databuf)
		} else {
			// get other data
		}
	}
	logp.Err("gse client recvMsgFromAgent quit")
}
