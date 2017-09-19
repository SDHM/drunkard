package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	grpclb "drunkard/x/sd/etcd"

	"drunkard/tests/pb"
)

var (
	serv  = flag.String("service", "hello_service", "service name")
	serva = flag.String("servicea", "serverA", "servicea name")
	servb = flag.String("serviceb", "serverB", "serviceb name")
	port  = flag.Int("port", 50001, "listening port")
	reg   = flag.String("reg", "http://127.0.0.1:2379", "register etcd address")
)

type tracingType struct{}

func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", *port))
	if err != nil {
		panic(err)
	}

	err = grpclb.Register(*serv, "127.0.0.1", *port, *reg, time.Second*5, 15)
	if err != nil {
		panic(err)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		s := <-ch
		log.Printf("receive signal '%v'", s)
		grpclb.UnRegister()
		os.Exit(1)
	}()

	log.Printf("starting hello service at %d", *port)

	s := grpc.NewServer()
	// s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})
	s.Serve(lis)
}

// server is used to implement helloworld.GreeterServer.
type server struct{}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	fmt.Printf("%v: Receive is %s\n", time.Now(), in.Name)
	time.Sleep(time.Second * 2)
	return &pb.HelloReply{Message: "From Server 31Hello " + in.Name}, nil
}
