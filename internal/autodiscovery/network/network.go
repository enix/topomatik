package network

import (
	"context"
	"errors"
	"fmt"

	"github.com/enix/topomatik/internal/logging"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

type Config struct {
	Interface string `yaml:"interface"`
}

type NetworkDiscoveryEngine struct {
	Config
}

func (n *NetworkDiscoveryEngine) Setup(_ context.Context) (err error) {
	return
}

func (n *NetworkDiscoveryEngine) Watch(ctx context.Context, callback func(data map[string]string, err error)) {
	routeUpdates := make(chan netlink.RouteUpdate, 16)
	if err := netlink.RouteSubscribe(routeUpdates, nil); err != nil {
		callback(nil, fmt.Errorf("failed to subscribe to route updates: %w", err))
		return
	}

	addrUpdates := make(chan netlink.AddrUpdate, 16)
	if err := netlink.AddrSubscribe(addrUpdates, nil); err != nil {
		callback(nil, fmt.Errorf("failed to subscribe to address updates: %w", err))
		return
	}

	n.emit(ctx, callback)

	for {
		select {
		case _, ok := <-routeUpdates:
			if !ok {
				callback(nil, errors.New("route subscription closed"))
				return
			}
		case _, ok := <-addrUpdates:
			if !ok {
				callback(nil, errors.New("address subscription closed"))
				return
			}
		}
		n.emit(ctx, callback)
	}
}

func (n *NetworkDiscoveryEngine) emit(ctx context.Context, callback func(data map[string]string, err error)) {
	log := logging.FromContext(ctx)

	link, err := n.getLink()
	if err != nil {
		callback(nil, err)
		return
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		callback(nil, err)
		return
	}

	if len(addrs) == 0 {
		callback(nil, fmt.Errorf("no IPv4 address on interface %q", link.Attrs().Name))
		return
	}

	ipnet := addrs[0].IPNet
	for _, addr := range addrs {
		if addr.Flags&unix.IFA_F_SECONDARY == 0 {
			ipnet = addr.IPNet
			break
		}
	}
	ones, _ := ipnet.Mask.Size()
	subnet := fmt.Sprintf("%s/%d", ipnet.IP.Mask(ipnet.Mask), ones)

	log.Debug("resolved subnet", "interface", link.Attrs().Name, "subnet", subnet)

	callback(map[string]string{
		"subnet": subnet,
	}, nil)
}

func (n *NetworkDiscoveryEngine) getLink() (netlink.Link, error) {
	if n.Interface == "" || n.Interface == "auto" {
		route, err := GetDefaultRoute(netlink.FAMILY_V4)
		if err != nil {
			return nil, err
		}
		return netlink.LinkByIndex(route.LinkIndex)
	}
	return netlink.LinkByName(n.Interface)
}

func GetDefaultRoute(family int) (*netlink.Route, error) {
	routes, err := netlink.RouteList(nil, family)
	if err != nil {
		return nil, err
	}

	for _, route := range routes {
		if route.Dst == nil {
			return &route, nil
		}

		ones, _ := route.Dst.Mask.Size()
		if route.Dst.IP.IsUnspecified() && ones == 0 {
			return &route, nil
		}
	}

	return nil, errors.New("default route not found")
}
