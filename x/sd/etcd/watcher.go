package etcd

import (
	"fmt"

	"time"

	"drunkard/x/lb"
	"drunkard/x/sd"

	etcd3 "github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc/naming"
)

// watcher is the implementaion of grpc.naming.Watcher
type watcher struct {
	re            *resolver // re: Etcd Resolver
	client        etcd3.Client
	isInitialized bool
}

// Close do nothing
func (w *watcher) Close() {
}

// Next to return the updates
func (w *watcher) Next() ([]*naming.Update, error) {
	// prefix is the etcd prefix/value to watch
	prefix := fmt.Sprintf("/%s/%s/", Prefix, w.re.serviceName)

	// check if is initialized
	if !w.isInitialized {
		// query addresses from etcd
		resp, err := w.client.Get(context.Background(), prefix, etcd3.WithPrefix())
		w.isInitialized = true
		if err == nil {
			addrs := extractAddrs(resp)
			//if not empty, return the updates or watcher new dir
			if l := len(addrs); l != 0 {
				updates := make([]*naming.Update, l)
				for i := range addrs {
					updates[i] = &naming.Update{Op: naming.Add, Addr: addrs[i].Addr, Metadata: addrs[i].Metadata}
				}
				return updates, nil
			}
		}
	}

	time.Sleep(time.Second)

	// generate etcd Watcher
	rch := w.client.Watch(context.Background(), prefix, etcd3.WithPrefix())
	for wresp := range rch {
		// fmt.Println("watch resp:", wresp)
		for _, ev := range wresp.Events {
			var reg sd.RegValue
			reg.Decode(ev.Kv.Value)
			switch ev.Type {
			case mvccpb.PUT:
				return []*naming.Update{{Op: naming.Add, Addr: reg.Addr, Metadata: string(ev.Kv.Value)}}, nil
			case mvccpb.DELETE:
				return []*naming.Update{{Op: naming.Delete, Addr: reg.Addr, Metadata: string(ev.Kv.Value)}}, nil
			}
		}
	}
	return nil, nil
}

func extractAddrs(resp *etcd3.GetResponse) []lb.Address {
	addrs := make([]lb.Address, 0)

	if resp == nil || resp.Kvs == nil {
		return addrs
	}

	for i := range resp.Kvs {
		if v := resp.Kvs[i].Value; v != nil {
			var reg sd.RegValue
			reg.Decode(v)

			addr := lb.Address{
				Addr:     reg.Addr,
				Metadata: string(v),
			}
			addrs = append(addrs, addr)

		}
	}

	return addrs
}
