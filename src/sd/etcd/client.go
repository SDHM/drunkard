package etcd

import (
	"sync"

	"github.com/coreos/etcd/clientv3"
)

var c *Client
var once sync.Once

type Client struct {
}

func GetClientInstance() *Client {

	once.Do(func() {
		c = new(Client)
	})

	return c
}

func (c *Client) Register(key string, value string) error {

	return nil
}

func (c *Client) UnRegister(key string) error {

	return nil
}

func (c *Client) Watch(withPrefix bool, prefix string) clientv3.WatchChan {

	return nil
}
