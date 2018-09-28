package storage

import (
	"errors"
	"time"
)

var _storage Storage
var ErrNotFound = errors.New("key not found")

type StorageConfig struct {
}

// Storage : key value storage
type Storage interface {
	Set(key, value string, expire time.Duration) error
	Get(key string) (string, error)
	Del(key string) error
	Close() error
}

// Init : init storage
func Init(path string, config *StorageConfig) error {
	var err error
	_storage, err = NewLocalStorage(path)
	return err
}

func Close() {
	_storage.Close()
}

func Get(key string) (string, error) {
	return _storage.Get(key)
}

// Set kv to storage, expire not used now
func Set(key, value string, expire time.Duration) error {
	if len(key) == 0 || len(value) == 0 {
		return nil
	}
	return _storage.Set(key, value, expire)
}

func Del(key string) error {
	return _storage.Del(key)
}
