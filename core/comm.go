package core

import (
	"errors"
	"fmt"
	"io"
	"log"

	"net"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"
)

// CommIPConn 定义了通用的IP连接接口
type CommIPConn interface {
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

// CommTCPConn 定义了TCP连接接口，继承自CommIPConn
type CommTCPConn interface {
	CommIPConn
	io.ReadWriteCloser
}

// CommUDPConn 定义了UDP连接接口，继承自CommIPConn
type CommUDPConn interface {
	CommIPConn
	io.ReadWriteCloser
}

// CommEndpoint 定义了通用的网络端点接口
type CommEndpoint interface {
	tcpip.Endpoint
}

const (
	tcpCongestionControlAlgorithm = "cubic" // TCP拥塞控制算法："reno" 或 "cubic"
)

// ForwarderCall 定义了TCP转发器的回调函数类型
type ForwarderCall func(conn CommTCPConn) error

// UdpForwarderCall 定义了UDP转发器的回调函数类型
type UdpForwarderCall func(conn CommUDPConn, ep CommEndpoint) error

// NewDefaultStack 创建并配置一个新的网络栈
func NewDefaultStack(mtu int, tcpCallback ForwarderCall) (*stack.Stack, *channel.Endpoint, error) {
	// 创建新的网络栈
	_netStack := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})

	// 启用IPv4和IPv6的转发
	_netStack.SetForwardingDefaultAndAllNICs(ipv4.ProtocolNumber, true)
	_netStack.SetForwardingDefaultAndAllNICs(ipv6.ProtocolNumber, true)

	var nicid tcpip.NICID = 1
	// 设置MAC地址
	macAddr, err := net.ParseMAC("de:ad:be:ee:ee:ef")
	if err != nil {
		fmt.Print(err.Error())
		return _netStack, nil, err
	}

	// 设置TCP拥塞控制算法
	opt1 := tcpip.CongestionControlOption(tcpCongestionControlAlgorithm)
	if err := _netStack.SetTransportProtocolOption(tcp.ProtocolNumber, &opt1); err != nil {
		return nil, nil, fmt.Errorf("set TCP congestion control algorithm: %s", err)
	}

	// 创建网络接口
	var channelLinkID = channel.New(1024, uint32(mtu), tcpip.LinkAddress(macAddr))
	if err := _netStack.CreateNIC(nicid, channelLinkID); err != nil {
		return _netStack, nil, errors.New(err.String())
	}

	// 设置路由表
	_netStack.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         nicid,
		},
	})

	// 启用混杂模式和IP欺骗
	_netStack.SetPromiscuousMode(nicid, true)
	_netStack.SetSpoofing(nicid, true)

	// 创建TCP转发器
	tcpForwarder := tcp.NewForwarder(_netStack, 0, 512, func(r *tcp.ForwarderRequest) {
		var wq waiter.Queue
		ep, err := r.CreateEndpoint(&wq)
		if err != nil {
			log.Printf("CreateEndpoint" + err.String() + "\r\n")
			r.Complete(true)
			return
		}
		defer ep.Close()
		r.Complete(false)
		if err := setKeepalive(ep); err != nil {
			log.Printf("setKeepalive" + err.Error() + "\r\n")
		}
		conn := gonet.NewTCPConn(&wq, ep)
		defer conn.Close()
		tcpCallback(conn)
	})

	// 设置TCP处理器
	_netStack.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)

	return _netStack, channelLinkID, nil
}

// setKeepalive 设置TCP保活选项
func setKeepalive(ep tcpip.Endpoint) error {
	idleOpt := tcpip.KeepaliveIdleOption(60 * time.Second)
	if err := ep.SetSockOpt(&idleOpt); err != nil {
		return fmt.Errorf("set keepalive idle: %s", err)
	}
	intervalOpt := tcpip.KeepaliveIntervalOption(30 * time.Second)
	if err := ep.SetSockOpt(&intervalOpt); err != nil {
		return fmt.Errorf("set keepalive interval: %s", err)
	}
	return nil
}
