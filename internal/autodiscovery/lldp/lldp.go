package lldp

import (
	"errors"
	"fmt"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

type LLDPDiscoveryEngine struct {
	Interface string

	tPacket *afpacket.TPacket
}

func (l *LLDPDiscoveryEngine) Setup() (err error) {
	if l.Interface == "" {
		if l.Interface, err = getDefaultRouteInterfaceName(); err != nil {
			return
		}
	}

	fmt.Println("lldp: using interface:", l.Interface)

	l.tPacket, err = afpacket.NewTPacket(
		afpacket.OptInterface(l.Interface),
		afpacket.OptFrameSize(afpacket.DefaultFrameSize),
		afpacket.OptBlockSize(afpacket.DefaultBlockSize),
		afpacket.OptNumBlocks(afpacket.DefaultNumBlocks),
		afpacket.OptBlockTimeout(afpacket.DefaultBlockTimeout),
		afpacket.OptPollTimeout(afpacket.DefaultPollTimeout),
	)
	if err != nil {
		return
	}

	var bpfFilter []bpf.RawInstruction
	bpfFilter, err = bpf.Assemble([]bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},                                  // Load EtherType
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: unix.ETH_P_LLDP, SkipFalse: 1}, // Only allow LLDP
		bpf.RetConstant{Val: 65535},                                         // Accept packet
		bpf.RetConstant{Val: 0},                                             // Drop
	})
	if err != nil {
		return
	}

	if err = l.tPacket.SetBPF(bpfFilter); err != nil {
		return
	}

	return
}

func (l *LLDPDiscoveryEngine) Watch(callback func(data map[string]string)) {
	packetSource := gopacket.NewPacketSource(l.tPacket, layers.LayerTypeEthernet)
	for packet := range packetSource.Packets() {
		if lldpLayer := packet.Layer(layers.LayerTypeLinkLayerDiscovery); lldpLayer != nil {
			data := map[string]string{}
			lldp, _ := lldpLayer.(*layers.LinkLayerDiscovery)

			for _, tlv := range lldp.Values {
				if tlv.Type == layers.LLDPTLVSysName {
					data["hostname"] = string(tlv.Value)
				}
				if tlv.Type == layers.LLDPTLVSysDescription {
					data["description"] = string(tlv.Value)
				}
			}

			callback(data)
		}
	}

	l.tPacket.Close()
}

func getDefaultRouteInterfaceName() (string, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return "", err
	}

	for _, route := range routes {
		ones, _ := route.Dst.Mask.Size()
		if route.Dst == nil || (route.Dst.IP.IsUnspecified() && ones == 0) {
			link, err := netlink.LinkByIndex(route.LinkIndex)
			if err != nil {
				return "", err
			}
			return link.Attrs().Name, nil
		}
	}

	return "", errors.New("default route not found")
}
