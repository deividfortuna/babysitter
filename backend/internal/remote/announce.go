package remote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/ipv4"
)

const ServiceType = "_babysitter._tcp.local."

const (
	mdnsPort         = 5353
	recordTTL        = 120
	legacyUnicastTTL = 10
	announceRepeats  = 3
	announcePause    = time.Second
	readBuffer       = 9000
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: mdnsPort}

type Announcement struct {
	Instance string
	Host     string
	Port     int
	Version  string
	Addr     netip.Addr
}

type announcer struct {
	queries      *net.UDPConn
	answers      *net.UDPConn
	service      dnsmessage.Name
	instance     dnsmessage.Name
	host         dnsmessage.Name
	a            Announcement
	addrs        func() []netip.Addr
	log          *slog.Logger
	group        *net.UDPAddr
	outgoing     []net.Interface
	useInterface func(net.Interface) error
	multicastMu  sync.Mutex
}

func Announce(ctx context.Context, a Announcement, log *slog.Logger) error {
	iface, err := a.boundInterface()
	if err != nil {
		return err
	}
	if bonjour, ok := systemBonjour(); ok {
		return announceWithBonjour(ctx, bonjour, a, iface)
	}
	return announceWithResponder(ctx, a, iface, log)
}

func (a Announcement) boundInterface() (*net.Interface, error) {
	if !a.Addr.IsValid() || a.Addr.IsUnspecified() {
		return nil, nil
	}
	if a.Addr.IsLoopback() {
		return nil, fmt.Errorf("the daemon listens on %v, which no other machine reaches", a.Addr)
	}
	if !a.Addr.Is4() {
		return nil, fmt.Errorf("the daemon listens on %v, and mDNS announces IPv4 addresses only", a.Addr)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if hasAddr(iface, a.Addr) {
			return &iface, nil
		}
	}
	return nil, fmt.Errorf("no network interface has the address %v", a.Addr)
}

func hasAddr(iface net.Interface, want netip.Addr) bool {
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if prefix, err := netip.ParsePrefix(addr.String()); err == nil && prefix.Addr() == want {
			return true
		}
	}
	return false
}

func systemBonjour() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	path, err := exec.LookPath("dns-sd")
	return path, err == nil
}

// macOS keeps the multicast that starts on the machine for mDNSResponder,
// so a daemon that answers alone is never found by an app on the same Mac.
func announceWithBonjour(ctx context.Context, bonjour string, a Announcement, iface *net.Interface) error {
	var args []string
	if iface != nil {
		args = append(args, "-i", iface.Name)
	}
	args = append(args, "-R", a.Instance, strings.TrimSuffix(ServiceType, ".local."), "local", strconv.Itoa(a.Port))
	args = append(args, a.txt()...)
	err := exec.CommandContext(ctx, bonjour, args...).Run()
	if err == nil || ctx.Err() != nil {
		return nil //nolint:nilerr // the context ends dns-sd, and that is how the daemon stops announcing
	}
	return fmt.Errorf("dns-sd: %w", err)
}

func announceWithResponder(ctx context.Context, a Announcement, iface *net.Interface, log *slog.Logger) error {
	queries, err := net.ListenMulticastUDP("udp4", iface, mdnsGroup)
	if err != nil {
		return fmt.Errorf("listen for mdns: %w", err)
	}
	defer queries.Close()
	answers, err := answerSocket(a, iface)
	if err != nil {
		return fmt.Errorf("open the mdns answer socket: %w", err)
	}
	defer answers.Close()
	an, err := newAnnouncer(queries, answers, a, log)
	if err != nil {
		return err
	}
	if iface == nil {
		an.outgoing = joinGroupOnLAN(queries, log)
	}
	go func() {
		<-ctx.Done()
		an.goodbye()
		queries.Close()
	}()
	go an.announce(ctx)
	an.serve()
	return nil
}

// The answers of a daemon bound to one address leave from that address
// and its interface, because the app takes the address of the answer.
func answerSocket(a Announcement, iface *net.Interface) (*net.UDPConn, error) {
	if iface == nil {
		return net.ListenUDP("udp4", &net.UDPAddr{})
	}
	conn, err := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(netip.AddrPortFrom(a.Addr, 0)))
	if err != nil {
		return nil, err
	}
	if err := ipv4.NewPacketConn(conn).SetMulticastInterface(iface); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (a Announcement) addrs() func() []netip.Addr {
	if !a.Addr.IsValid() || a.Addr.IsUnspecified() {
		return LANAddrs
	}
	return func() []netip.Addr { return []netip.Addr{a.Addr} }
}

func newAnnouncer(queries, answers *net.UDPConn, a Announcement, log *slog.Logger) (*announcer, error) {
	service, err := dnsmessage.NewName(ServiceType)
	if err != nil {
		return nil, err
	}
	instance, err := dnsmessage.NewName(label(a.Instance) + "." + ServiceType)
	if err != nil {
		return nil, err
	}
	host, err := dnsmessage.NewName(label(a.Host) + ".local.")
	if err != nil {
		return nil, err
	}
	an := &announcer{queries: queries, answers: answers, service: service, instance: instance, host: host, a: a, addrs: a.addrs(), log: log, group: mdnsGroup}
	an.useInterface = func(iface net.Interface) error {
		return ipv4.NewPacketConn(an.answers).SetMulticastInterface(&iface)
	}
	return an, nil
}

// A socket that joins the group with no interface hears only the one the
// system picks, so a daemon on all addresses joins on each LAN interface.
func joinGroupOnLAN(queries *net.UDPConn, log *slog.Logger) []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	conn := ipv4.NewPacketConn(queries)
	var joined []net.Interface
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil || !canAnnounceOn(iface, addrs) {
			continue
		}
		if err := conn.JoinGroup(&iface, mdnsGroup); err != nil {
			log.Debug("mdns: join the group", "interface", iface.Name, "err", err)
		}
		joined = append(joined, iface)
	}
	return joined
}

func canAnnounceOn(iface net.Interface, addrs []net.Addr) bool {
	carriesMulticast := isLAN(iface) && iface.Flags&net.FlagMulticast != 0
	return carriesMulticast && slices.ContainsFunc(addrs, func(a net.Addr) bool {
		_, ok := ipv4Of(a)
		return ok
	})
}

func label(name string) string {
	cleaned := strings.Trim(strings.NewReplacer(".", "-", " ", "-").Replace(strings.TrimSpace(name)), "-")
	if cleaned == "" {
		return "babysitter"
	}
	return cleaned
}

func (an *announcer) announce(ctx context.Context) {
	for range announceRepeats {
		an.multicast(recordTTL)
		select {
		case <-ctx.Done():
			return
		case <-time.After(announcePause):
		}
	}
}

func (an *announcer) goodbye() {
	an.multicast(0)
}

func (an *announcer) serve() {
	buf := make([]byte, readBuffer)
	for {
		n, from, err := an.queries.ReadFromUDP(buf)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				an.log.Debug("mdns: read", "err", err)
			}
			return
		}
		an.answer(buf[:n], from)
	}
}

func (an *announcer) answer(packet []byte, from *net.UDPAddr) {
	var p dnsmessage.Parser
	header, err := p.Start(packet)
	if err != nil || header.Response {
		return
	}
	questions, err := p.AllQuestions()
	if err != nil {
		return
	}
	asked := an.askedFor(questions)
	if asked == nil {
		return
	}
	if from.Port == mdnsPort {
		an.multicast(recordTTL)
		return
	}
	an.send(from, header.ID, asked, legacyUnicastTTL)
}

func (an *announcer) askedFor(questions []dnsmessage.Question) []dnsmessage.Question {
	for _, q := range questions {
		if an.serves(q.Name) {
			return []dnsmessage.Question{{Name: q.Name, Type: q.Type, Class: dnsmessage.ClassINET}}
		}
	}
	return nil
}

func (an *announcer) serves(name dnsmessage.Name) bool {
	asked := strings.ToLower(name.String())
	served := []dnsmessage.Name{an.service, an.instance, an.host}
	return slices.ContainsFunc(served, func(n dnsmessage.Name) bool { return asked == strings.ToLower(n.String()) })
}

// The announcer, the goodbye and the answers to queries all multicast, and
// each one sets the outgoing interface of the shared socket before it writes.
func (an *announcer) multicast(ttl uint32) {
	an.multicastMu.Lock()
	defer an.multicastMu.Unlock()
	if len(an.outgoing) == 0 {
		an.send(an.group, 0, nil, ttl)
		return
	}
	for _, iface := range an.outgoing {
		if err := an.useInterface(iface); err != nil {
			an.log.Debug("mdns: send on interface", "interface", iface.Name, "err", err)
			continue
		}
		an.send(an.group, 0, nil, ttl)
	}
}

func (an *announcer) send(to *net.UDPAddr, id uint16, questions []dnsmessage.Question, ttl uint32) {
	msg, err := an.response(id, questions, ttl)
	if err != nil {
		an.log.Warn("mdns: build answer", "err", err)
		return
	}
	if _, err := an.answers.WriteToUDP(msg, to); err != nil {
		an.log.Debug("mdns: send answer", "to", to.String(), "err", err)
	}
}

func (an *announcer) response(id uint16, questions []dnsmessage.Question, ttl uint32) ([]byte, error) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, Response: true, Authoritative: true})
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	for _, q := range questions {
		if err := b.Question(q); err != nil {
			return nil, err
		}
	}
	if err := b.StartAnswers(); err != nil {
		return nil, err
	}
	header := func(name dnsmessage.Name) dnsmessage.ResourceHeader {
		return dnsmessage.ResourceHeader{Name: name, Class: dnsmessage.ClassINET, TTL: ttl}
	}
	if err := b.PTRResource(header(an.service), dnsmessage.PTRResource{PTR: an.instance}); err != nil {
		return nil, err
	}
	if err := b.SRVResource(header(an.instance), dnsmessage.SRVResource{Target: an.host, Port: uint16(an.a.Port)}); err != nil { //nolint:gosec // a TCP port fits in 16 bits
		return nil, err
	}
	if err := b.TXTResource(header(an.instance), dnsmessage.TXTResource{TXT: an.a.txt()}); err != nil {
		return nil, err
	}
	for _, addr := range an.addrs() {
		if err := b.AResource(header(an.host), dnsmessage.AResource{A: addr.As4()}); err != nil {
			return nil, err
		}
	}
	return b.Finish()
}

func (a Announcement) txt() []string {
	return []string{
		"name=" + a.Instance,
		"host=" + a.Host,
		"port=" + strconv.Itoa(a.Port),
		"version=" + a.Version,
	}
}

func LANAddrs() []netip.Addr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, iface := range ifaces {
		if !isLAN(iface) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ip, ok := ipv4Of(a); ok {
				out = append(out, ip)
			}
		}
	}
	return out
}

func isLAN(iface net.Interface) bool {
	up := iface.Flags&net.FlagUp != 0
	loopback := iface.Flags&net.FlagLoopback != 0
	return up && !loopback
}

func ipv4Of(a net.Addr) (netip.Addr, bool) {
	prefix, err := netip.ParsePrefix(a.String())
	if err != nil {
		return netip.Addr{}, false
	}
	ip := prefix.Addr()
	return ip, ip.Is4() && !ip.IsLinkLocalUnicast()
}
