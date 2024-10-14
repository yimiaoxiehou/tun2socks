package core

import (
	"context"
	"fmt"

	"io"
	"log"
	"net"

	"github.com/yimiaoxiehou/tun2socks/socks"
	"github.com/yimiaoxiehou/tun2socks/tun"

	"gvisor.dev/gvisor/pkg/buffer"

	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

var wrapnet uint32
var mask uint32
var relayip net.IP
var port uint16
var sock5Addr string

func StartTunDevice(tunDevice string, tunAddr string, tunMask string, mtu int, _sock5Addr string) {
	dev, err := tun.RegTunDev(tunDevice, tunAddr, tunMask)
	if err != nil {
		fmt.Println("start tun err:", err)
		return
	}
	sock5Addr = _sock5Addr
	ForwardTransportFromIo(dev, mtu, rawTcpForwarder)
}

func rawTcpForwarder(conn CommTCPConn) error {
	defer conn.Close()
	socksConn, err1 := socks.NewConn(sock5Addr)
	if err1 != nil {
		log.Println(err1)
		return nil
	}
	defer socksConn.Close()

	if socks.SocksCmd(socksConn, uint8(socks.SOCKS5_CONNECT_CMD), conn.LocalAddr().String()) == nil {
		go io.Copy(socksConn, conn)
		io.Copy(conn, socksConn)
	}
	return nil
}

func ForwardTransportFromIo(dev io.ReadWriteCloser, mtu int, tcpCallback ForwarderCall) error {
	_, channelLinkID, err := NewDefaultStack(mtu, tcpCallback)
	if err != nil {
		log.Printf("err:%v", err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// write tun
	go func(_ctx context.Context) {
		for {
			info := channelLinkID.ReadContext(_ctx)
			if info == nil {
				log.Printf("channelLinkID exit \r\n")
				break
			}
			view := info.ToView()
			view.WriteTo(dev)
			info.DecRef()
		}
	}(ctx)

	// read tun data
	var buf = make([]byte, mtu+80)
	var recvLen = 0
	for {
		recvLen, err = dev.Read(buf[:])
		if err != nil {
			log.Printf("err:%v", err)
			break
		}

		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(buf[:recvLen]),
		})

		switch header.IPVersion(buf) {
		case header.IPv4Version:
			channelLinkID.InjectInbound(header.IPv4ProtocolNumber, pkt)
		case header.IPv6Version:
			channelLinkID.InjectInbound(header.IPv6ProtocolNumber, pkt)
		}
		pkt.DecRef()
	}
	return nil
}
