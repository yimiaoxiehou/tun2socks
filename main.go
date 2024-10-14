package main

import (
	"flag"
)

var tunDevice = flag.String("dev", "demo-tun", "tunDevice name")
var tunAddr = flag.String("addr", "192.168.124.211", "tunAddr 192.168.124.211")
var netmask = flag.String("mask", "255.255.255.0", "mask 255.255.255.0")
var mtu = flag.Int("mtu", 1420, "mtu 1420")
var socksAddr = flag.String("proxy", "socks5://127.0.0.1:1080", "socksAddr")

func main() {
	flag.Parse()

	StartTunDevice(*tunDevice, *tunAddr, *netmask, *mtu, *socksAddr)
}
