package sd

import (
	"encoding/json"
)

type RegValue struct {
	Version string `json:"version"`  // 版本
	Addr    string `json:"addr"`     // 地址 IP:PORT
	Weight  string `json:"weight"`   // 权重
	HashCoe string `json:"hashCode"` // 节点哈希值
	MaxCnn  string `json:"maxcnn"`   // 最大连接数
}

func (r *RegValue) Decode(buf []byte) error {
	return json.Unmarshal(buf, r)
}

func (r *RegValue) Encode() string {
	if buf, err := json.Marshal(r); nil != err {
		return ""
	} else {
		return string(buf)
	}
}
