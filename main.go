package main

import (
	"flag"

	"github.com/yimiaoxiehou/tun2socks/tun2socks"
)

var tunDevice = flag.String("dev", "demo-tun", "tunDevice name")
var tunAddr = flag.String("addr", "192.168.124.211", "tunAddr 192.168.124.211")
var mask = flag.String("mask", "255.255.255.0", "mask 255.255.255.0")
var mtu = flag.Int("mtu", 1420, "mtu 1420")
var socksAddr = flag.String("proxy", "socks5://127.0.0.1:1080", "socksAddr")

func main() {
	flag.Parse()

	tun2socks.StartTunDevice(*tunDevice, *tunAddr, *mask, *mtu, *socksAddr)
}
