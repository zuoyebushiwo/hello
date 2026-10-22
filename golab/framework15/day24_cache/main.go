package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"frameworklab/internal/labkit"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"
)

type UserCache struct {
	cache *expirable.LRU[string, string]
	group singleflight.Group
	loads atomic.Int64
}

func NewUserCache(size int, ttl time.Duration) *UserCache {
	return &UserCache{cache: expirable.NewLRU[string, string](size, nil, ttl)}
}

func (c *UserCache) Get(ctx context.Context, key string, loader func(context.Context) (string, error)) (string, bool, error) {
	if value, ok := c.cache.Get(key); ok {
		return value, true, nil
	}
	value, err, _ := c.group.Do(key, func() (any, error) {
		if cached, ok := c.cache.Get(key); ok {
			return cached, nil
		}
		c.loads.Add(1)
		loaded, err := loader(ctx)
		if err != nil {
			return "", err
		}
		c.cache.Add(key, loaded)
		return loaded, nil
	})
	if err != nil {
		return "", false, err
	}
	return value.(string), false, nil
}

func (c *UserCache) Loads() int64 { return c.loads.Load() }
func (c *UserCache) Len() int     { return c.cache.Len() }

func main() {
	lab := labkit.New("Day 24", "缓存：LRU、TTL 与 singleflight 防击穿")
	ctx := context.Background()
	cache := NewUserCache(2, 50*time.Millisecond)

	lab.Section("场景 A：首次回源，后续命中")
	value, hit, err := cache.Get(ctx, "user:1", func(context.Context) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "alice", nil
	})
	lab.Infof("value=%s hit=%v loads=%d err=%v", value, hit, cache.Loads(), err)
	lab.Check(err == nil && !hit, "第一次未命中，执行 loader")
	value, hit, err = cache.Get(ctx, "user:1", func(context.Context) (string, error) {
		return "unexpected", nil
	})
	lab.Check(err == nil && hit && value == "alice" && cache.Loads() == 1, "第二次命中，不再访问后端")

	lab.Section("场景 B：容量上限触发 LRU 淘汰")
	cache = NewUserCache(2, time.Minute)
	_, _, _ = cache.Get(ctx, "a", func(context.Context) (string, error) { return "A", nil })
	_, _, _ = cache.Get(ctx, "b", func(context.Context) (string, error) { return "B", nil })
	_, _, _ = cache.Get(ctx, "a", func(context.Context) (string, error) { return "A", nil })
	_, _, _ = cache.Get(ctx, "c", func(context.Context) (string, error) { return "C", nil })
	_, hitA, _ := cache.Get(ctx, "a", func(context.Context) (string, error) { return "A", nil })
	_, hitB, _ := cache.Get(ctx, "b", func(context.Context) (string, error) { return "B", nil })
	lab.Infof("len=%d hitA=%v hitB=%v", cache.Len(), hitA, hitB)
	lab.Check(cache.Len() == 2 && hitA && !hitB, "LRU 淘汰最久未使用的 key")

	lab.Section("场景 C：TTL 过期")
	cache = NewUserCache(2, 20*time.Millisecond)
	_, _, _ = cache.Get(ctx, "ttl", func(context.Context) (string, error) { return "old", nil })
	time.Sleep(30 * time.Millisecond)
	value, hit, _ = cache.Get(ctx, "ttl", func(context.Context) (string, error) { return "new", nil })
	lab.Infof("value=%s hit=%v", value, hit)
	lab.Check(!hit && value == "new", "TTL 过期后重新加载")

	lab.Section("场景 D：并发回源只执行一次")
	cache = NewUserCache(10, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = cache.Get(ctx, "hot", func(context.Context) (string, error) {
				time.Sleep(20 * time.Millisecond)
				return "hot-value", nil
			})
		}()
	}
	wg.Wait()
	lab.Infof("concurrent=50 loads=%d", cache.Loads())
	lab.Check(cache.Loads() == 1, "缓存击穿被 singleflight 合并")

	lab.Section("选型边界")
	lab.Infof("本地缓存：hashicorp/golang-lru、ristretto、bigcache；按命中率、淘汰策略、内存成本选")
	lab.Infof("分布式缓存：Redis + 本地二级缓存；必须处理一致性、穿透、雪崩和热 key")
	lab.Infof("不要缓存可变权限、余额等强一致数据，除非有版本/失效协议")
	lab.Done()
}
