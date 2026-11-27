package kube

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

func ParseDockerSubnets(out string) []string {
	var subnets []string
	for _, f := range strings.Fields(out) {
		if _, _, err := net.ParseCIDR(f); err == nil {
			subnets = append(subnets, f)
		}
	}
	return subnets
}

func MetalLBRange(subnets []string) (string, error) {
	for _, s := range subnets {
		from, to, err := metalLBPool(s)
		if err != nil {
			continue
		}
		return from + "-" + to, nil
	}
	if len(subnets) == 0 {
		return "", fmt.Errorf("no subnets")
	}
	return "", fmt.Errorf("no usable IPv4 range in %s", strings.Join(subnets, " "))
}

func metalLBPool(cidr string) (string, string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", err
	}
	ip := ipnet.IP.To4()
	if ip == nil {
		return "", "", fmt.Errorf("not ipv4")
	}
	ones, bits := ipnet.Mask.Size()
	if bits != 32 || ones < 0 {
		return "", "", fmt.Errorf("not ipv4")
	}
	mask := net.CIDRMask(ones, 32)
	start := binary.BigEndian.Uint32(ip.Mask(mask))
	last := start | ^binary.BigEndian.Uint32(mask)
	if last <= start+1 {
		return "", "", fmt.Errorf("subnet too small")
	}
	base := last & 0xffffff00
	a, b := base+200, base+250
	if a <= start || b >= last {
		b = last - 1
		a = base + 200
		if a <= start || a > b {
			const want = 51
			if b >= start+want {
				a = b - want + 1
			} else {
				a = start + 1
			}
		}
	}
	if a <= start {
		a = start + 1
	}
	if b >= last {
		b = last - 1
	}
	if a > b {
		return "", "", fmt.Errorf("no room in %s", cidr)
	}
	return ip4(a), ip4(b), nil
}

func ip4(n uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return net.IP(b[:]).String()
}

func MetalLBManifest(namespace, pool, fromTo string) string {
	if namespace == "" {
		namespace = "metallb-system"
	}
	if pool == "" {
		pool = "k2-kind"
	}
	return "apiVersion: metallb.io/v1beta1\nkind: IPAddressPool\nmetadata:\n  name: " + pool +
		"\n  namespace: " + namespace + "\nspec:\n  addresses:\n  - " + fromTo +
		"\n---\napiVersion: metallb.io/v1beta1\nkind: L2Advertisement\nmetadata:\n  name: " + pool +
		"\n  namespace: " + namespace + "\nspec:\n  ipAddressPools:\n  - " + pool + "\n"
}

func DockerNetworkInspectArgs(network string) []string {
	if network == "" {
		network = "kind"
	}
	return []string{"network", "inspect", "-f", "{{range .IPAM.Config}}{{.Subnet}} {{end}}", network}
}
