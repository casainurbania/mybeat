// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

import "time"

type Config struct {
	DataID int32         `config:"dataid"`
	Period time.Duration `config:"period"`
}

var DefaultConfig = Config{
	DataID: 0,
	Period: 1 * time.Minute,
}
