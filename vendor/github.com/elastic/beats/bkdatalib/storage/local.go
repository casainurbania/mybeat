// +build linux windows

package storage

import (
	"time"

	"github.com/rapidloop/skv"
)

type LocalStorage struct {
	store *skv.KVStore
}

// New : skv storage
func NewLocalStorage(path string) (*LocalStorage, error) {
	store, err := skv.Open(path)
	return &LocalStorage{store: store}, err
}

// Close : close db
func (cli *LocalStorage) Close() error {
	return cli.store.Close()
}

// Set : set value
func (cli *LocalStorage) Set(key, value string, expire time.Duration) error {
	return cli.store.Put(key, value)
}

// Get : get value
func (cli *LocalStorage) Get(key string) (val string, err error) {
	err = cli.store.Get(key, &val)
	if err == skv.ErrNotFound {
		err = ErrNotFound
	}
	return val, err
}

// Del : delete key
func (cli *LocalStorage) Del(key string) error {
	return cli.store.Delete(key)
}
