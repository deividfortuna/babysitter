package remote

import (
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/deividfortuna/babysitter/internal/testutil"
)

func startAnnouncer(t *testing.T) *net.UDPAddr {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	an, err := newAnnouncer(conn, conn, Announcement{Instance: "studio server", Host: "studio", Port: 7420, Version: "1.2.3"}, testutil.Logger(t))
	if err != nil {
		t.Fatal(err)
	}
	an.addrs = func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.168.1.20")} }
	go an.serve()
	t.Cleanup(func() { conn.Close() })
	return conn.LocalAddr().(*net.UDPAddr)
}

func query(t *testing.T, name string) []byte {
	t.Helper()
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 42})
	if err := b.StartQuestions(); err != nil {
		t.Fatal(err)
	}
	q := dnsmessage.Question{Name: dnsmessage.MustNewName(name), Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}
	if err := b.Question(q); err != nil {
		t.Fatal(err)
	}
	msg, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func ask(t *testing.T, to *net.UDPAddr, name string) ([]dnsmessage.Resource, bool) {
	t.Helper()
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.WriteToUDP(query(t, name), to); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReadDeadline(time.Now().Add(300 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, readBuffer)
	n, err := client.Read(buf)
	if err != nil {
		return nil, false
	}
	var msg dnsmessage.Message
	if err := msg.Unpack(buf[:n]); err != nil {
		t.Fatal(err)
	}
	if msg.ID != 42 || !msg.Response {
		t.Fatalf("answer header %+v, want a response to query 42", msg.Header)
	}
	return msg.Answers, true
}

func TestAnnouncerAnswersAQueryForTheService(t *testing.T) {
	addr := startAnnouncer(t)

	answers, ok := ask(t, addr, ServiceType)
	if !ok {
		t.Fatal("no answer to a query for the service")
	}
	var txt []string
	var port uint16
	var ptr, a string
	for _, rr := range answers {
		switch body := rr.Body.(type) {
		case *dnsmessage.PTRResource:
			ptr = body.PTR.String()
		case *dnsmessage.SRVResource:
			port = body.Port
		case *dnsmessage.TXTResource:
			txt = body.TXT
		case *dnsmessage.AResource:
			a = netip.AddrFrom4(body.A).String()
		}
	}
	if ptr != "studio-server."+ServiceType {
		t.Fatalf("PTR = %q", ptr)
	}
	if port != 7420 || a != "192.168.1.20" {
		t.Fatalf("SRV port %d, A %s", port, a)
	}
	for _, want := range []string{"name=studio server", "port=7420", "version=1.2.3", "host=studio"} {
		if !slices.Contains(txt, want) {
			t.Fatalf("TXT %v lacks %q", txt, want)
		}
	}
}

func TestAnnouncerIgnoresOtherServices(t *testing.T) {
	addr := startAnnouncer(t)

	if _, ok := ask(t, addr, "_airplay._tcp.local."); ok {
		t.Fatal("the announcer answered a query for another service")
	}
}
