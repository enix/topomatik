package lldp

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/bpf"
)

type LLDPDiscoveryService struct {
	Interface string

	tPacket *afpacket.TPacket
}

func (l *LLDPDiscoveryService) Watch() (dataChannel chan map[string]string, err error) {
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

	dataChannel = make(chan map[string]string)
	go l.HandlePackets(dataChannel)

	return
}

func (l *LLDPDiscoveryService) HandlePackets(dataChannel chan map[string]string) {
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

			dataChannel <- data
		}
	}

	l.tPacket.Close()
}
