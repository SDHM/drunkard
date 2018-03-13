package main

import (
	"flag"
	"fmt"
	"time"

	"strconv"

	"drunkard/tests/pb"

	"drunkard/x/lb/ch"
	grpclb "drunkard/x/sd/etcd"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	serv = flag.String("service", "hello_service", "service name")
	reg  = flag.String("reg", "http://127.0.0.1:2379", "register etcd address")
)

const (
	// Our service name.
	serviceName = "zipkinclient1"

	// Host + port of our service.
	hostPort = "127.0.0.1:50000"

	// Endpoint to send Zipkin spans to.
	zipkinHTTPEndpoint = "http://localhost:9411/api/v1/spans"

	// Debug mode.
	debug = false

	// Base endpoint of our SVC1 service.
	svc1Endpoint = "http://localhost:61001"

	// same span can be set to true for RPC style spans (Zipkin V1) vs Node style (OpenTracing)
	sameSpan = true
)

func main() {
	flag.Parse()

	r := grpclb.NewResolver(*serv)
	b := ch.ConsistentHaslLB(r)

	// ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	// Create Root Span for duration of the interaction with svc1

	conn, err := grpc.DialContext(context.Background(), *reg, grpc.WithInsecure(), grpc.WithBalancer(b))
	if err != nil {
		panic(err)
	}
	client := pb.NewGreeterClient(conn)

	ticker := time.NewTicker(time.Millisecond * 200)
	count := 0
	for t := range ticker.C {

		count++;
		ctx := context.WithValue(context.Background(), "userid", count)
	
		resp, err := client.SayHello(ctx, &pb.HelloRequest{Name: "world " + strconv.Itoa(t.Second())})
		if err == nil {
			fmt.Printf("%v: Reply is %s\n", t, resp.Message)
		} else {
			fmt.Println(err.Error())
		}
	}
}
