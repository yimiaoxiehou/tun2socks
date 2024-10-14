package socks

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var SOCKS5_CONNECT_CMD = 0x01
var SOCKS5_BIND_CMD = 0x02
var SOCKS5_UDP_ASSOCIATE_CMD = 0x03

func NewConn(sock5Addr string) (net.Conn, error) {
	parsedURL, err := url.Parse(sock5Addr)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// 提取主机和端口
	host := parsedURL.Hostname()
	port := parsedURL.Port()
	if port == "" {
		port = "1080"
	}

	// 建立到SOCKS5代理服务器的连接
	socksConn, err := net.Dial("tcp", host+":"+port)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// 发送SOCKS5握手请求
	// 0x05: SOCKS5版本
	// 0x01: 支持的认证方法数量
	// 0x00: 不需要认证方法
	socksConn.Write([]byte{0x05, 0x01, 0x00})

	// 读取服务器的认证响应
	authBack := make([]byte, 2)
	_, err = io.ReadFull(socksConn, authBack)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	// 从URL中提取用户名和密码
	username := parsedURL.User.Username()
	password, _ := parsedURL.User.Password()

	// 检查服务器选择的认证方法
	if authBack[1] == 0x02 {
		// 服务器要求用户名/密码认证
		if username == "" || password == "" {
			return nil, fmt.Errorf("socks5 username and password is empty")
		}

		// 构造用户名/密码认证请求
		auth := []byte{0x01} // 认证子协商版本
		auth = append(auth, byte(len(username)))
		auth = append(auth, []byte(username)...)
		auth = append(auth, byte(len(password)))
		auth = append(auth, []byte(password)...)

		// 发送认证请求
		socksConn.Write(auth)

		// 读取认证响应
		authResponse := make([]byte, 2)
		_, err = io.ReadFull(socksConn, authResponse)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		// 检查认证是否成功
		if authResponse[1] != 0x00 {
			return nil, fmt.Errorf("authentication failed")
		}
	} else if authBack[1] != 0x00 {
		// 服务器不接受无认证方法，也不接受用户名/密码认证
		return nil, fmt.Errorf("no acceptable authentication methods")
	}

	// 认证成功，返回已建立的连接
	return socksConn, nil
}

/*to socks5*/
// SocksCmd 发送SOCKS5命令到代理服务器
// socksConn: 与SOCKS5服务器建立的连接
// cmd: SOCKS5命令（例如：0x01表示CONNECT）
// host: 目标主机地址，格式为"IP:端口"
func SocksCmd(socksConn net.Conn, cmd uint8, host string) error {
	// 解析目标主机地址
	hosts := strings.Split(host, ":")
	rAddr := net.ParseIP(hosts[0])
	_port, _ := strconv.Atoi(hosts[1])

	// 构造SOCKS5请求头
	// 0x05: SOCKS版本号
	// cmd: 命令（如CONNECT）
	// 0x00: 保留字段
	// 0x01: IPv4地址类型
	msg := []byte{0x05, cmd, 0x00, 0x01}
	buffer := bytes.NewBuffer(msg)

	// 写入目标IP地址（4字节）
	binary.Write(buffer, binary.BigEndian, rAddr.To4())

	// 写入目标端口（2字节）
	binary.Write(buffer, binary.BigEndian, uint16(_port))

	// 发送SOCKS5请求到服务器
	socksConn.Write(buffer.Bytes())

	// 读取服务器响应
	conectBack := make([]byte, 10)
	_, err := io.ReadFull(socksConn, conectBack)
	if err != nil {
		log.Println("读取SOCKS5服务器响应失败:", err)
		return err
	}

	// TODO: 解析服务器响应，检查是否成功建立连接

	return nil
}
