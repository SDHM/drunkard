package lb

import (
	"sync"

	"sync/atomic"

	"fmt"

	"google.golang.org/grpc"
)

// 与地址相关的信息
type AddrInfo struct {
	Addr          grpc.Address // 地址
	Connected     bool         // 是否连接
	Weight        int          // 权重
	CurrentWeight int          // 当前权重
	Active        int32        // 活跃连接数
	Mu            sync.Mutex   //
}

func (ai *AddrInfo) Lock() {
	ai.Mu.Lock()
}

func (ai *AddrInfo) UnLock() {
	ai.Mu.Unlock()
}

func (ai *AddrInfo) AddActive() {
	atomic.AddInt32(&ai.Active, 1)
	fmt.Printf("current activity of:%s count:%d\n", ai.Addr.Addr, ai.Active)
}

func (ai *AddrInfo) SubActive() {
	atomic.AddInt32(&ai.Active, -1)
	fmt.Printf("current activity of:%s count:%d\n", ai.Addr.Addr, ai.Active)
}
