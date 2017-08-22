package sd

/**
 *	服务发现订阅者接口
 *
 *
 */

import (
	"github.com/coreos/etcd/clientv3"
)

type ISubScriber interface {
	Watcher(service string) clientv3.WatchChan
}
