package sd

import (
	"fmt"
	"testing"
)

func Test_Reg(t *testing.T) {
	r := RegValue{
		Version: "1.1.1",
		Addr:    "localhost:11211",
		Weight:  "1000",
		MaxCnn:  10000,
		HashCoe: "123",
	}

	str := r.Encode()

	var g RegValue
	g.Decode([]byte(str))

	fmt.Println("g.Encode:", g.Encode())
}
