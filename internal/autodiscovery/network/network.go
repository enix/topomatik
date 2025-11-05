package network

import (
	"errors"
	"fmt"
	"time"

	"github.com/vishvananda/netlink"
)

type Config struct {
	Interval  time.Duration `yaml:"interval" validate:"required"`
	Interface string        `yaml:"interface"`
}

type NetworkDiscoveryEngine struct {
	Config
}

func (n *NetworkDiscoveryEngine) Setup() (err error) {
	return
}

func (n *NetworkDiscoveryEngine) Watch(callback func(data map[string]string, err error)) {
	var route *netlink.Route
	var err error

	ticker := time.NewTicker(n.Interval)

	for {
		if n.Interface == "" {
			route, err = GetDefaultRoute()
		} else {
			route, err = getRouteByInterfaceName(n.Interface)
		}

		if err != nil {
			callback(nil, err)
			return
		}

		if route.Family != netlink.FAMILY_V4 {
			callback(nil, fmt.Errorf("unsupported IP family (code: %d), must be IPV4", route.Family))
			return
		}

		ones, _ := route.Src.DefaultMask().Size()
		callback(map[string]string{
			"subnet": fmt.Sprintf("%s/%d", route.Src.Mask(route.Src.DefaultMask()), ones),
		}, nil)

		<-ticker.C
	}
}

func GetDefaultRoute() (*netlink.Route, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
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

func getRouteByInterfaceName(interfaceName string) (*netlink.Route, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, err
	}

	for _, route := range routes {
		link, err := netlink.LinkByIndex(route.LinkIndex)
		if err != nil {
			return nil, err
		}

		if link.Attrs().Name == interfaceName {
			return &route, nil
		}
	}

	return nil, fmt.Errorf("route with interface name \"%s\" not found", interfaceName)
}
