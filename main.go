package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/yimiaoxiehou/tun2socks/core"
)

var tunDevice = flag.String("dev", "demo-tun", "tunDevice name")
var tunAddr = flag.String("addr", "192.168.124.211", "tunAddr 192.168.124.211")
var netmask = flag.String("mask", "255.255.255.0", "mask 255.255.255.0")
var tunGW = flag.String("gw", "192.168.124.1", "gw 192.168.124.1")
var tunDNS = flag.String("dns", "127.0.0.1", "dns 127.0.0.1")
var mtu = flag.Int("mtu", 1420, "mtu 1420")
var socksAddr = flag.String("proxy", "socks5://127.0.0.1:1080", "socksAddr")

func main() {
	flag.Parse()

	e := &core.Engine{
		TunDevice: *tunDevice,
		TunAddr:   *tunAddr,
		TunMask:   *netmask,
		TunGW:     *tunGW,
		TunDNS:    *tunDNS,
		Mtu:       *mtu,
		Sock5Addr: *socksAddr,
	}
	go func() {
		err := e.Start()
		if err != nil {
			fmt.Println(err)
		}
	}()
	time.Sleep(1000 * time.Second)
}
