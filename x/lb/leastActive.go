package lb

import (
	"drunkard/x/sd"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"golang.org/x/net/context"

	"math/rand"

	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/naming"
)

type leastActive struct {
	r         naming.Resolver
	w         naming.Watcher
	addrs     []*addrInfo         // all the addresses the client should potentially connect
	mu        sync.Mutex          //
	addrCh    chan []grpc.Address // the channel to notify gRPC internals the list of addresses the client should connect to.
	next      int                 // index of the next address to return for Get()
	waitCh    chan struct{}       // the channel to block when there is no connected address available
	done      bool                // 负载均衡是否关闭
	weightSum int                 // 权重总数
	rd        *rand.Rand
}

func LeastActiveLB(r naming.Resolver) grpc.Balancer {
	return &leastActive{r: r}
}

func (la *leastActive) Start(target string, config grpc.BalancerConfig) error {
	la.rd = rand.New(rand.NewSource(time.Now().UnixNano()))
	la.mu.Lock()
	defer la.mu.Unlock()
	if la.done {
		return grpc.ErrClientConnClosing
	}
	if la.r == nil {
		// If there is no name resolver installed, it is not needed to
		// do name resolution. In this case, target is added into la.addrs
		// as the only address available and la.addrCh stays nil.
		la.addrs = append(la.addrs, &addrInfo{addr: grpc.Address{Addr: target}})
		return nil
	}
	w, err := la.r.Resolve(target)
	if err != nil {
		return err
	}
	la.w = w
	la.addrCh = make(chan []grpc.Address, 1)
	go func() {
		for {
			if err := la.watchAddrUpdates(); err != nil {
				return
			}
		}
	}()
	return nil
}

func (la *leastActive) watchAddrUpdates() error {
	updates, err := la.w.Next()
	if err != nil {
		grpclog.Warningf("grpc: the naming watcher stops working due to %v.", err)
		return err
	}
	la.mu.Lock()
	defer la.mu.Unlock()
	for _, update := range updates {
		addr := grpc.Address{
			Addr:     update.Addr,
			Metadata: update.Metadata,
		}
		switch update.Op {
		case naming.Add:
			var exist bool
			for _, v := range la.addrs {
				if addr == v.addr {
					exist = true
					// 如果存在更新权重
					var reg sd.RegValue
					reg.Decode([]byte(update.Metadata.(string)))
					v.weight, _ = strconv.Atoi(reg.Weight)
					grpclog.Infoln("grpc: The name resolver wanted to add an existing address: ", addr)
					break
				}
			}
			if exist {
				continue
			} else {
				var reg sd.RegValue
				reg.Decode([]byte(update.Metadata.(string)))
				weight, _ := strconv.Atoi(reg.Weight)
				la.weightSum += weight
				la.addrs = append(la.addrs, &addrInfo{addr: addr, weight: weight})
			}

		case naming.Delete:
			for i, v := range la.addrs {
				if addr == v.addr {
					copy(la.addrs[i:], la.addrs[i+1:])
					la.weightSum -= la.addrs[i].weight
					la.addrs = la.addrs[:len(la.addrs)-1]
					break
				}
			}
		default:
			grpclog.Errorln("Unknown update.Op ", update.Op)
		}
	}
	// Make a copy of la.addrs and write it onto la.addrCh so that gRPC internals gets notified.
	open := make([]grpc.Address, len(la.addrs))
	for i, v := range la.addrs {
		open[i] = v.addr
	}
	if la.done {
		return grpc.ErrClientConnClosing
	}
	select {
	case <-la.addrCh:
	default:
	}
	la.addrCh <- open
	return nil
}

func (la *leastActive) Up(addr grpc.Address) func(error) {

	la.mu.Lock()
	defer la.mu.Unlock()
	var cnt int
	for _, a := range la.addrs {
		if a.addr == addr {
			if a.connected {
				return nil
			}
			a.connected = true
		}
		if a.connected {
			cnt++
		}
	}
	// addr is only one which is connected. Notify the Get() callers who are blocking.
	if cnt == 1 && la.waitCh != nil {
		close(la.waitCh)
		la.waitCh = nil
	}
	return func(err error) {
		la.down(addr, err)
	}
}

// down unsets the connected state of addr.
func (la *leastActive) down(addr grpc.Address, err error) {
	la.mu.Lock()
	defer la.mu.Unlock()
	for _, a := range la.addrs {
		if addr == a.addr {
			a.connected = false
			break
		}
	}
}

// Get returns the next addr in the rotation.
func (la *leastActive) Get(ctx context.Context, opts grpc.BalancerGetOptions) (addr grpc.Address, put func(), err error) {

	var ch chan struct{}
	la.mu.Lock()
	if la.done {
		la.mu.Unlock()
		err = grpc.ErrClientConnClosing
		return
	}

	if len(la.addrs) > 0 {
		if la.next >= len(la.addrs) {
			la.next = 0
		}
		next := la.next
		for {
			a := la.addrs[next]
			next = la.getNext()
			if a.connected {
				addr = a.addr
				la.next = next
				a.AddActive()
				put = func() {
					a.Lock()
					a.SubActive()
					a.UnLock()
					fmt.Println("rpc call return")
				}
				la.mu.Unlock()
				return
			}
			if next == la.next {
				// Has iterated all the possible address but none is connected.
				break
			}
		}
	}
	if !opts.BlockingWait {
		if len(la.addrs) == 0 {
			la.mu.Unlock()
			err = grpc.Errorf(codes.Unavailable, "there is no address available")
			return
		}

		// Returns the next addr on la.addrs for failfast RPCs.
		addr = la.addrs[la.next].addr
		la.addrs[la.next].AddActive()
		put = func() {
			la.addrs[la.next].Lock()
			la.addrs[la.next].SubActive()
			la.addrs[la.next].UnLock()
			fmt.Println("rpc call return")
		}
		la.next = la.getNext()
		la.mu.Unlock()
		fmt.Println("return2:", addr.Addr)
		return
	}
	// Wait on la.waitCh for non-failfast RPCs.
	if la.waitCh == nil {
		ch = make(chan struct{})
		la.waitCh = ch
	} else {
		ch = la.waitCh
	}
	la.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		case <-ch:
			la.mu.Lock()
			if la.done {
				la.mu.Unlock()
				err = grpc.ErrClientConnClosing
				return
			}

			if len(la.addrs) > 0 {

				if la.next >= len(la.addrs) {
					la.next = 0
				}
				next := la.next
				for {
					a := la.addrs[next]
					next = la.getNext()
					if a.connected {
						addr = a.addr
						la.next = next
						a.AddActive()
						put = func() {
							a.Lock()
							a.SubActive()
							a.UnLock()
							fmt.Println("rpc call return")
						}
						la.mu.Unlock()
						fmt.Println("return3:", addr.Addr)
						return
					}
					if next == la.next {
						// Has iterated all the possible address but none is connected.
						break
					}
				}
			}
			// The newly added addr got removed by Down() again.
			if la.waitCh == nil {
				ch = make(chan struct{})
				la.waitCh = ch
			} else {
				ch = la.waitCh
			}
			la.mu.Unlock()
		}
	}
}

func (la *leastActive) Notify() <-chan []grpc.Address {
	return la.addrCh
}

func (la *leastActive) Close() error {
	la.mu.Lock()
	defer la.mu.Unlock()
	if la.done {
		return errors.New("grpc: balancer is closed")
	}
	la.done = true
	if la.w != nil {
		la.w.Close()
	}
	if la.waitCh != nil {
		close(la.waitCh)
		la.waitCh = nil
	}
	if la.addrCh != nil {
		close(la.addrCh)
	}
	return nil
}

func (la *leastActive) getNext() int {
	index := -1
	size := len(la.addrs)
	leastNum := la.addrs[0].active
	leastIndex := make([]int, 0)
	for i := 0; i < size; i++ {
		active := la.addrs[i].active
		if leastNum > active {
			leastNum = active
		}
	}

	for i := 0; i < size; i++ {
		if la.addrs[i].active == leastNum {
			leastIndex = append(leastIndex, i)
		}
	}

	index = leastIndex[la.rd.Intn(len(leastIndex))]

	return index
}
