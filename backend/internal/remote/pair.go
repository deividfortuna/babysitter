package remote

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var osHostname = os.Hostname

func Hostname() string {
	name, err := osHostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "babysitter"
	}
	return label(strings.TrimSuffix(strings.TrimSuffix(name, "."), ".local"))
}

func PairingLinks(listenHost string, port int, token string) []string {
	if ip := net.ParseIP(listenHost); ip != nil && !ip.IsUnspecified() {
		return []string{PairingLink(ip.String(), port, token)}
	}
	hosts := []string{Hostname() + ".local"}
	for _, addr := range LANAddrs() {
		hosts = append(hosts, addr.String())
	}
	links := make([]string, 0, len(hosts))
	for _, host := range hosts {
		links = append(links, PairingLink(host, port, token))
	}
	return links
}

func PairingLink(host string, port int, token string) string {
	link := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/",
		Fragment: url.Values{TokenQueryParam: {token}}.Encode(),
	}
	return link.String()
}
