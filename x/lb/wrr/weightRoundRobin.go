package wrr

import (
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc/codes"

	"drunkard/x/sd"

	"strconv"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/naming"
	"drunkard/x/lb"
)

type weightRoundRobin struct {
	r         naming.Resolver
	w         naming.Watcher
	addrs     []*lb.AddrInfo         // all the addresses the client should potentially connect
	mu        sync.Mutex          //
	addrCh    chan []grpc.Address // the channel to notify gRPC internals the list of addresses the client should connect to.
	next      int                 // index of the next address to return for Get()
	waitCh    chan struct{}       // the channel to block when there is no connected address available
	done      bool                // 负载均衡是否关闭
	weightSum int                 // 权重总数
}

func WeightRoundRobin(r naming.Resolver) grpc.Balancer {
	return &weightRoundRobin{r: r}
}

func (rr *weightRoundRobin) Start(target string, config grpc.BalancerConfig) error {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if rr.done {
		return grpc.ErrClientConnClosing
	}
	if rr.r == nil {
		// If there is no name resolver installed, it is not needed to
		// do name resolution. In this case, target is added into rr.addrs
		// as the only address available and rr.addrCh stays nil.
		rr.addrs = append(rr.addrs, &lb.AddrInfo{Addr: grpc.Address{Addr: target}})
		return nil
	}
	w, err := rr.r.Resolve(target)
	if err != nil {
		return err
	}
	rr.w = w
	rr.addrCh = make(chan []grpc.Address, 1)
	go func() {
		for {
			if err := rr.watchAddrUpdates(); err != nil {
				return
			}
		}
	}()
	return nil
}

func (rr *weightRoundRobin) watchAddrUpdates() error {
	updates, err := rr.w.Next()
	if err != nil {
		grpclog.Warningf("grpc: the naming watcher stops working due to %v.", err)
		return err
	}
	rr.mu.Lock()
	defer rr.mu.Unlock()
	for _, update := range updates {
		addr := grpc.Address{
			Addr:     update.Addr,
			Metadata: update.Metadata,
		}
		switch update.Op {
		case naming.Add:
			var exist bool
			for _, v := range rr.addrs {
				if addr == v.Addr {
					exist = true
					// 如果存在更新权重
					var reg sd.RegValue
					reg.Decode([]byte(update.Metadata.(string)))
					v.Weight, _ = strconv.Atoi(reg.Weight)
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
				rr.weightSum += weight
				rr.addrs = append(rr.addrs, &lb.AddrInfo{Addr: addr, Weight: weight})
			}

		case naming.Delete:
			for i, v := range rr.addrs {
				if addr == v.Addr {
					copy(rr.addrs[i:], rr.addrs[i+1:])
					rr.weightSum -= rr.addrs[i].Weight
					rr.addrs = rr.addrs[:len(rr.addrs)-1]
					break
				}
			}
		default:
			grpclog.Errorln("Unknown update.Op ", update.Op)
		}
	}
	// Make a copy of rr.addrs and write it onto rr.addrCh so that gRPC internals gets notified.
	open := make([]grpc.Address, len(rr.addrs))
	for i, v := range rr.addrs {
		open[i] = v.Addr
	}
	if rr.done {
		return grpc.ErrClientConnClosing
	}
	select {
	case <-rr.addrCh:
	default:
	}
	rr.addrCh <- open
	return nil
}

func (rr *weightRoundRobin) Up(addr grpc.Address) func(error) {

	rr.mu.Lock()
	defer rr.mu.Unlock()
	var cnt int
	for _, a := range rr.addrs {
		if a.Addr == addr {
			if a.Connected {
				return nil
			}
			a.Connected = true
		}
		if a.Connected {
			cnt++
		}
	}
	// addr is only one which is connected. Notify the Get() callers who are blocking.
	if cnt == 1 && rr.waitCh != nil {
		close(rr.waitCh)
		rr.waitCh = nil
	}
	return func(err error) {
		rr.down(addr, err)
	}
}

// down unsets the connected state of addr.
func (rr *weightRoundRobin) down(addr grpc.Address, err error) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	for _, a := range rr.addrs {
		if addr == a.Addr {
			a.Connected = false
			break
		}
	}
}

// Get returns the next addr in the rotation.
func (rr *weightRoundRobin) Get(ctx context.Context, opts grpc.BalancerGetOptions) (addr grpc.Address, put func(), err error) {
	put = func() {
		fmt.Println("rpc call return")
	}

	var ch chan struct{}
	rr.mu.Lock()
	if rr.done {
		rr.mu.Unlock()
		err = grpc.ErrClientConnClosing
		return
	}

	if len(rr.addrs) > 0 {
		if rr.next >= len(rr.addrs) {
			rr.next = 0
		}
		next := rr.next
		for {
			a := rr.addrs[next]
			next = lb_getNext(rr.addrs, len(rr.addrs))
			if a.Connected {
				addr = a.Addr
				rr.next = next
				rr.mu.Unlock()
				return
			}
			if next == rr.next {
				// Has iterated all the possible address but none is connected.
				break
			}
		}
	}
	if !opts.BlockingWait {
		if len(rr.addrs) == 0 {
			rr.mu.Unlock()
			err = grpc.Errorf(codes.Unavailable, "there is no address available")
			return
		}

		// Returns the next addr on rr.addrs for failfast RPCs.
		addr = rr.addrs[rr.next].Addr
		rr.next = lb_getNext(rr.addrs, len(rr.addrs))
		rr.mu.Unlock()
		fmt.Println("return2:", addr.Addr)
		return
	}
	// Wait on rr.waitCh for non-failfast RPCs.
	if rr.waitCh == nil {
		ch = make(chan struct{})
		rr.waitCh = ch
	} else {
		ch = rr.waitCh
	}
	rr.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		case <-ch:
			rr.mu.Lock()
			if rr.done {
				rr.mu.Unlock()
				err = grpc.ErrClientConnClosing
				return
			}

			if len(rr.addrs) > 0 {

				if rr.next >= len(rr.addrs) {
					rr.next = 0
				}
				next := rr.next
				for {
					a := rr.addrs[next]
					next = lb_getNext(rr.addrs, len(rr.addrs))
					if a.Connected {
						addr = a.Addr
						rr.next = next
						rr.mu.Unlock()
						fmt.Println("return3:", addr.Addr)
						return
					}
					if next == rr.next {
						// Has iterated all the possible address but none is connected.
						break
					}
				}
			}
			// The newly added addr got removed by Down() again.
			if rr.waitCh == nil {
				ch = make(chan struct{})
				rr.waitCh = ch
			} else {
				ch = rr.waitCh
			}
			rr.mu.Unlock()
		}
	}
}

func (rr *weightRoundRobin) Notify() <-chan []grpc.Address {
	return rr.addrCh
}

func (rr *weightRoundRobin) Close() error {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if rr.done {
		return errors.New("grpc: balancer is closed")
	}
	rr.done = true
	if rr.w != nil {
		rr.w.Close()
	}
	if rr.waitCh != nil {
		close(rr.waitCh)
		rr.waitCh = nil
	}
	if rr.addrCh != nil {
		close(rr.addrCh)
	}
	return nil
}

func lb_getNext(ss []*lb.AddrInfo, size int) int {

	index := -1
	total := 0
	for i := 0; i < size; i++ {
		ss[i].CurrentWeight += ss[i].Weight
		total += ss[i].Weight
		if index == -1 || ss[index].CurrentWeight < ss[i].CurrentWeight {
			index = i
		}
	}

	ss[index].CurrentWeight -= total
	return index
}
