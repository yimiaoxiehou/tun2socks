package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/yimiaoxiehou/tun2socks/core"
)

var tunDevice = flag.String("dev", "demo-tun", "tunDevice name")
var tunAddr = flag.String("addr", "10.10.10.10", "tunAddr 10.10.10.10")
var netmask = flag.String("mask", "255.255.255.255", "mask 255.255.255.255")
var mtu = flag.Int("mtu", 1420, "mtu 1420")
var socksAddr = flag.String("proxy", "socks5://192.168.44.213:1080", "socksAddr")
var routers = flag.String("routers", "10.10.10.0/24", "routers router1,router2,router3")

func main() {
	flag.Parse()

	e := &core.Engine{
		TunDevice: *tunDevice,
		TunAddr:   *tunAddr,
		TunMask:   *netmask,
		Mtu:       *mtu,
		Sock5Addr: *socksAddr,
		Routers:   strings.Split(*routers, ","),
	}
	go func() {
		err := e.Start()
		if err != nil {
			fmt.Println(err)
		}
	}()
	time.Sleep(1000 * time.Second)
}
