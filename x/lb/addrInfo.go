package lb

import (
	"sync"

	"sync/atomic"

	"fmt"

	"google.golang.org/grpc"
)

// 与地址相关的信息
type addrInfo struct {
	addr          grpc.Address // 地址
	connected     bool         // 是否连接
	weight        int          // 权重
	currentWeight int          // 当前权重
	active        int32        // 活跃连接数
	mu            sync.Mutex   //
}

func (ai *addrInfo) Lock() {
	ai.mu.Lock()
}

func (ai *addrInfo) UnLock() {
	ai.mu.Unlock()
}

func (ai *addrInfo) AddActive() {
	atomic.AddInt32(&ai.active, 1)
	fmt.Printf("current activity of:%s count:%d\n", ai.addr.Addr, ai.active)
}

func (ai *addrInfo) SubActive() {
	atomic.AddInt32(&ai.active, -1)
	fmt.Printf("current activity of:%s count:%d\n", ai.addr.Addr, ai.active)
}
