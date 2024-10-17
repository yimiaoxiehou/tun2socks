package core

import (
	"context"

	"io"
	"log"

	"github.com/yimiaoxiehou/tun2socks/socks"
	"github.com/yimiaoxiehou/tun2socks/tun"

	"gvisor.dev/gvisor/pkg/buffer"

	"sync"

	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type Engine struct {
	TunDevice string
	TunAddr   string
	TunMask   string
	TunGW     string
	TunDNS    string
	Mtu       int
	Sock5Addr string
	dev       io.ReadWriteCloser
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func (e *Engine) Start() error {
	var err error
	e.dev, err = tun.RegTunDev(e.TunDevice, e.TunAddr, e.TunMask, e.TunGW, e.TunDNS)
	if err != nil {
		return err
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		err := e.ForwardTransportFromIo(e.ctx, e.dev, e.rawTcpForwarder)
		if err != nil && err != context.Canceled {
			log.Printf("ForwardTransportFromIo error: %v", err)
		}
	}()
	return nil
}

func (e *Engine) Stop() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	if e.dev != nil {
		err := e.dev.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) rawTcpForwarder(conn CommTCPConn) error {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing CommTCPConn: %v", err)
		}
	}()

	socksConn, err := socks.NewConn(e.Sock5Addr)
	if err != nil {
		log.Printf("Error creating SOCKS connection: %v", err)
		return err
	}
	defer func() {
		if err := socksConn.Close(); err != nil {
			log.Printf("Error closing SOCKS connection: %v", err)
		}
	}()

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

	// Wait for both goroutines to finish
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil && err != io.EOF {
			log.Printf("Error in data transfer: %v", err)
			return err
		}
	}

	return nil
}

func (e *Engine) ForwardTransportFromIo(ctx context.Context, dev io.ReadWriter, tcpCallback ForwarderCall) error {
	_, channelLinkID, err := NewDefaultStack(e.Mtu, tcpCallback)
	if err != nil {
		log.Printf("err:%v", err)
		return err
	}

	// write tun
	go func(_ctx context.Context) {
		for {
			info := channelLinkID.ReadContext(_ctx)
			if info == nil {
				log.Printf("channelLinkID exit \r\n")
				break
			}
			view := info.ToView()
			view.WriteTo(e.dev)
			info.DecRef()
		}
	}(ctx)

	// read tun data
	var buf = make([]byte, e.Mtu+80)
	var recvLen = 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			recvLen, err = dev.Read(buf[:])
			if err != nil {
				if err == io.EOF || err == io.ErrClosedPipe {
					return nil // Normal closure
				}
				return err
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
	}
}
