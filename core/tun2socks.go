package core

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"

	"io"

	"github.com/yimiaoxiehou/tun2socks/socks"
	"github.com/yimiaoxiehou/tun2socks/tun"

	"gvisor.dev/gvisor/pkg/buffer"

	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type Engine struct {
	TunDevice string
	TunAddr   string
	TunMask   string
	Mtu       int
	Sock5Addr string
	Routers   []string
	dev       io.ReadWriteCloser
	ctx       context.Context
	cancel    context.CancelFunc
}

// Start initializes and starts the tun2socks engine.
func (e *Engine) Start() error {
	var err error

	log.Println("Start")

	// Register and initialize the TUN device
	e.dev, err = tun.RegTunDev(e.TunDevice, e.Mtu, e.TunAddr, e.TunMask, e.Routers)
	if err != nil {
		return err // Return error if TUN device initialization fails
	}

	// Create a cancellable context for the engine
	e.ctx, e.cancel = context.WithCancel(context.Background())

	// Start the main processing goroutine
	go func() {
		// Ensure the wait group counter is decremented when the goroutine exits

		// Start forwarding transport from IO
		err := e.ForwardTransportFromIo(e.ctx, e.dev, e.rawTcpForwarder, e.rawUdpForwarder)

		// Log any errors that occur during forwarding, except for context cancellation
		if err != nil && err != context.Canceled {
			log.Printf("ForwardTransportFromIo error: %v", err)
		}
	}()

	return nil // Return nil if startup was successful
}

func (e *Engine) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	if e.dev != nil {
		err := e.dev.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) rawUdpForwarder(conn CommUDPConn, ep CommEndpoint) error {
	defer conn.Close()
	//dns port
	if strings.HasSuffix(conn.LocalAddr().String(), ":53") {
		dnsReq(conn, "udp", "127.0.0.1:53")
	}
	return nil
}

func (e *Engine) rawTcpForwarder(conn CommTCPConn) error {
	socksConn, err := socks.NewConn(e.Sock5Addr)
	if err != nil {
		log.Printf("Error creating SOCKS connection: %v", err)
		return err
	}
	defer func() {
		log.Println("Close conn " + conn.LocalAddr().String())
		if err := conn.Close(); err != nil && err != io.EOF {
			log.Printf("Error closing CommTCPConn: %v", err)
		}
		if err := socksConn.Close(); err != nil && err != io.EOF {
			log.Printf("Error closing SOCKS connection: %v", err)
		}
	}()

	log.Println("connect to " + conn.LocalAddr().String())
	if err := socks.SocksCmd(socksConn, uint8(socks.SOCKS5_CONNECT_CMD), conn.LocalAddr().String()); err != nil {
		log.Printf("SOCKS command failed: %v", err)
		return err
	}

	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(socksConn, conn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(conn, socksConn)
		errChan <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			log.Printf("Error in data transfer: %v", err)
		}
	}
	return nil
}

func (e *Engine) ForwardTransportFromIo(ctx context.Context, dev io.ReadWriter, tcpCallback ForwarderCall, udpCallback UdpForwarderCall) error {
	_, channelLinkID, err := NewDefaultStack(e.Mtu, tcpCallback, udpCallback)
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
			info.ToView().WriteTo(dev)
			info.DecRef()
		}
	}(ctx)

	// read tun data
	var buf = make([]byte, e.Mtu+80)
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

/*to dns*/
func dnsReq(conn CommUDPConn, action string, dnsAddr string) error {
	if action == "tcp" {
		dnsConn, err := net.Dial(action, dnsAddr)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		defer dnsConn.Close()
		go io.Copy(conn, dnsConn)
		io.Copy(dnsConn, conn)
		fmt.Printf("dnsReq Tcp\r\n")
		return nil
	} else {
		buf := make([]byte, 4096)
		var n = 0
		var err error
		n, err = conn.Read(buf)
		if err != nil {
			fmt.Printf("c.Read() = %v", err)
			return err
		}
		dnsConn, err := net.Dial("udp", dnsAddr)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		defer dnsConn.Close()
		_, err = dnsConn.Write(buf[:n])
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		n, err = dnsConn.Read(buf)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		_, err = conn.Write(buf[:n])
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
	}
	return nil
}
