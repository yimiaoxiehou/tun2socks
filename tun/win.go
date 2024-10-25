//go:build windows
// +build windows

package tun

import (
	"crypto/md5"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"unsafe"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
	_ "golang.zx2c4.com/wireguard/windows/tunnel"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

//go:embed wintun/amd64/wintun.dll
var winAmd64Bin []byte

//go:embed wintun/x86/wintun.dll
var winX86Bin []byte

//go:embed wintun/arm/wintun.dll
var winArmBin []byte

//go:embed wintun/arm64/wintun.dll
var winArm64Bin []byte

func init() {
	var dllBin []byte
	var dllPath = "C:\\Windows\\System32\\wintun.dll"

	switch runtime.GOARCH {
	case "amd64":
		dllBin = winAmd64Bin
		break
	case "x86":
		dllBin = winX86Bin
		break
	case "arm":
		dllBin = winArmBin
		break
	case "arm64":
		dllBin = winArm64Bin
		break
	}
	_, err := os.Stat(dllPath)
	if err != nil && len(dllBin) > 0 {
		os.WriteFile(dllPath, dllBin, os.ModePerm)
	} else {
		//
		oldBin, err := os.ReadFile(dllPath)
		if err != nil {
			return
		}
		oldMd5 := fmt.Sprintf("%x", md5.Sum(oldBin))
		newMd5 := fmt.Sprintf("%x", md5.Sum(dllBin))
		//update
		if oldMd5 != newMd5 {
			os.WriteFile(dllPath, dllBin, os.ModePerm)
		}
	}
}

/*windows use wintun*/
func RegTunDev(tunDevice string, mtu int, tunAddr string, tunMask string, routers []string) (*DevReadWriteCloser, error) {
	if len(tunDevice) == 0 {
		tunDevice = "socksTun0"
	}
	if len(tunAddr) == 0 {
		tunAddr = "10.0.0.2"
	}
	if len(tunMask) == 0 {
		tunMask = "255.255.255.0"
	}
	tunDev, err := tun.CreateTUN(tunDevice, mtu)
	if err != nil {
		return nil, err
	}
	setInterfaceAddress4(tunDev.(*tun.NativeTun), tunAddr, tunMask)
	for _, router := range routers {
		CmdHide("route", "add", router, tunAddr).Run()
	}
	return &DevReadWriteCloser{tunDev.(*tun.NativeTun)}, nil
}

func setInterfaceAddress4(tunDev *tun.NativeTun, addr, mask string) error {
	luid := winipcfg.LUID(tunDev.LUID())
	ipnet := net.IPNet{
		IP:   net.ParseIP(addr).To4(),
		Mask: net.IPMask(net.ParseIP(mask).To4()),
	}
	addresses := append([]netip.Prefix{}, netip.MustParsePrefix(ipnet.String()))
	err := luid.SetIPAddressesForFamily(windows.AF_INET, addresses)
	if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
		cleanupAddressesOnDisconnectedInterfaces(windows.AF_INET, addresses)
		err = luid.SetIPAddressesForFamily(windows.AF_INET, addresses)
	}
	return err
}

func CmdHide(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

func determineGUID(name string) *windows.GUID {
	b := make([]byte, unsafe.Sizeof(windows.GUID{}))
	if _, err := io.ReadFull(hkdf.New(md5.New, []byte(name), nil, nil), b); err != nil {
		return nil
	}
	return (*windows.GUID)(unsafe.Pointer(&b[0]))
}

//go:linkname cleanupAddressesOnDisconnectedInterfaces golang.zx2c4.com/wireguard/windows/tunnel.cleanupAddressesOnDisconnectedInterfaces
func cleanupAddressesOnDisconnectedInterfaces(family winipcfg.AddressFamily, addresses []netip.Prefix)
