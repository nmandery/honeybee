package honeybee

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"github.com/peterbourgon/diskv"
	"hash/crc32"
	"io"
)

// ForgettingCache is an implementation of httpcache.Cache that supplements the in-memory map with persistent storage
type ForgettingCache struct {
	d *diskv.Diskv

	// how many percent of the keys should be "forgotten"
	// during on call of ForgetSome
	forgetPercent int

	// counter to step through the subsets of the cache contents
	// to delete
	forgetCounter int
}

// Get returns the response corresponding to key if present
func (c *ForgettingCache) Get(key string) (resp []byte, ok bool) {
	key = keyToFilename(key)
	resp, err := c.d.Read(key)
	if err != nil {
		return []byte{}, false
	}
	return resp, true
}

// Set saves a response to the cache as key
func (c *ForgettingCache) Set(key string, resp []byte) {
	key = keyToFilename(key)
	c.d.WriteStream(key, bytes.NewReader(resp), true)
}

// Delete removes the response with key from the cache
func (c *ForgettingCache) Delete(key string) {
	key = keyToFilename(key)
	c.d.Erase(key)
}

// Drop a few entries from the cache, calling this function
// will drop a few - more or less random - keys from the cache.
// Call it repeatedly and all entires will be dropped.
func (c *ForgettingCache) ForgetSome() {
	modValue := 1
	if c.forgetPercent > 0 && c.forgetPercent <= 100 {
		modValue = 100 / c.forgetPercent
	}

	for key := range c.d.Keys(nil) {
		hashCRC32 := int(crc32.ChecksumIEEE([]byte(key)))
		if (hashCRC32 % modValue) == c.forgetCounter {
			c.d.Erase(key)
		}
	}

	c.forgetCounter++
	if c.forgetCounter == modValue {
		c.forgetCounter = 0
	}
}

func keyToFilename(key string) string {
	h := md5.New()
	io.WriteString(h, key)
	return hex.EncodeToString(h.Sum(nil))
}

// NewWithDiskv returns a new Cache using the provided Diskv as underlying
// storage.
// forgetPercent: how many percent of the keys should be "forgotten" during on
// call of ForgetSome. Use 100 to delete all keys, 50 to delete half of them, ...
func NewForgettingCache(d *diskv.Diskv, forgetPercent int) *ForgettingCache {
	return &ForgettingCache{
		d:             d,
		forgetPercent: forgetPercent,
		forgetCounter: 0,
	}
}
