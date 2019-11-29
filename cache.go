package sudu

import (
	"sync"
	"time"

	gcache "github.com/patrickmn/go-cache"
)

// 简单的实现了一个cache
// 用于sudu的内建cache，调用者可以使用自己的cache
var defaultCaches map[string]*gcache.Cache = map[string]*gcache.Cache{}
var cacheLock *sync.Mutex = &sync.Mutex{}

type Cache struct {
	Cache *gcache.Cache
	Key   string
}

func NewCache(ns, key string) *Cache {
	return &Cache{
		Cache: GetCacheBucket(ns),
		Key:   key,
	}
}

func (c *Cache) Set(nvs []interface{}) {
	c.Cache.SetDefault(c.Key, nvs)
}

func (c *Cache) Get() []interface{} {
	v, ok := c.Cache.Get(c.Key)
	if ok {
		return v.([]interface{})
	}
	return make([]interface{}, 0)
}

func (c *Cache) Delete() {
	c.Cache.Delete(c.Key)
}

func GetCacheBucket(ns string) *gcache.Cache {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	gc := defaultCaches[ns]
	if gc == nil {
		// 24h ok
		// every 1h exec cache cleaner
		gc = gcache.New(24*time.Hour, 1*time.Hour)
		defaultCaches[ns] = gc
	}

	return gc
}
