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

func NewDefaultStack(mtu int, tcpCallback ForwarderCall, udpCallback UdpForwarderCall) (*stack.Stack, *channel.Endpoint, error) {

	// Generate unique NIC id.

	_netStack := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})

	macAddr, _ := net.ParseMAC("de:ad:be:ee:ee:ef")

	var nicid tcpip.NICID = 1
	var linkID stack.LinkEndpoint
	var channelLinkID = channel.New(1024, uint32(mtu), tcpip.LinkAddress(macAddr))
	linkID = channelLinkID
	if err := _netStack.CreateNIC(nicid, linkID); err != nil {
		return _netStack, nil, errors.New(err.String())
	}
	_netStack.CreateNICWithOptions(nicid, linkID,
		stack.NICOptions{
			Disabled: false,
			QDisc:    nil,
		})

	_netStack.SetPromiscuousMode(nicid, true)
	_netStack.SetSpoofing(nicid, true)

	_netStack.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         nicid,
		},
		{
			Destination: header.IPv6EmptySubnet,
			NIC:         nicid,
		},
	})

	tcpForwarder := tcp.NewForwarder(_netStack, 30000, 10, func(r *tcp.ForwarderRequest) {
		var wq waiter.Queue
		ep, err := r.CreateEndpoint(&wq)
		if err != nil {
			log.Printf("CreateEndpoint" + err.String() + "\r\n")
			r.Complete(true)
			return
		}
		r.Complete(false)
		setSocketOptions(_netStack, ep)
		conn := gonet.NewTCPConn(&wq, ep)
		tcpCallback(conn)
	})
	_netStack.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpForwarder.HandlePacket)

	udpForwarder := udp.NewForwarder(_netStack, func(r *udp.ForwarderRequest) {
		var wq waiter.Queue
		ep, err := r.CreateEndpoint(&wq)
		if err != nil {
			log.Printf("r.CreateEndpoint() = %v", err)
			return
		}
		go udpCallback(gonet.NewUDPConn(&wq, ep), ep)
	})
	_netStack.SetTransportProtocolHandler(udp.ProtocolNumber, udpForwarder.HandlePacket)

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

func setSocketOptions(s *stack.Stack, ep tcpip.Endpoint) tcpip.Error {
	{ /* TCP keepalive options */
		ep.SocketOptions().SetKeepAlive(true)

		idle := tcpip.KeepaliveIdleOption(60 * time.Second)
		if err := ep.SetSockOpt(&idle); err != nil {
			return err
		}

		interval := tcpip.KeepaliveIntervalOption(30 * time.Second)
		if err := ep.SetSockOpt(&interval); err != nil {
			return err
		}

		if err := ep.SetSockOptInt(tcpip.KeepaliveCountOption, 9); err != nil {
			return err
		}
	}
	{ /* TCP recv/send buffer size */
		var ss tcpip.TCPSendBufferSizeRangeOption
		if err := s.TransportProtocolOption(header.TCPProtocolNumber, &ss); err == nil {
			ep.SocketOptions().SetSendBufferSize(int64(ss.Default), false)
		}

		var rs tcpip.TCPReceiveBufferSizeRangeOption
		if err := s.TransportProtocolOption(header.TCPProtocolNumber, &rs); err == nil {
			ep.SocketOptions().SetReceiveBufferSize(int64(rs.Default), false)
		}
	}
	return nil
}
