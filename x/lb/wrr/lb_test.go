package wrr

import (
	"testing"

	"fmt"

	"google.golang.org/grpc"
	"drunkard/x/lb"
)

func Test_Lb(t *testing.T) {
	addr1 := lb.addrInfo{
		addr: grpc.Address{
			Addr: "a",
		},
		weight: 4,
	}

	addr2 := lb.addrInfo{
		addr: grpc.Address{
			Addr: "b",
		},
		weight: 2,
	}

	addr3 := lb.addrInfo{
		addr: grpc.Address{
			Addr: "c",
		},
		weight: 1,
	}

	addrs := make([]*lb.addrInfo, 0)
	addrs = append(addrs, &addr1)
	addrs = append(addrs, &addr2)
	addrs = append(addrs, &addr3)

	for i := 0; i < 11; i++ {
		index := lb_getNext(addrs, 3)
		fmt.Println("index:", index+1)
	}

}
