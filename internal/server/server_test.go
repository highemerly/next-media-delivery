package server

import (
	"bufio"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewSetsTimeouts(t *testing.T) {
	s := New(Config{Host: "127.0.0.1", Port: 0, AdminPort: 0, WriteTimeout: 2 * time.Minute})
	for name, srv := range map[string]*http.Server{"proxy": s.proxy, "admin": s.admin} {
		if srv.ReadHeaderTimeout != defaultReadHeaderTimeout {
			t.Errorf("%s ReadHeaderTimeout = %s, want %s", name, srv.ReadHeaderTimeout, defaultReadHeaderTimeout)
		}
		if srv.ReadTimeout != defaultReadTimeout {
			t.Errorf("%s ReadTimeout = %s, want %s", name, srv.ReadTimeout, defaultReadTimeout)
		}
	}
	if s.proxy.WriteTimeout != 2*time.Minute {
		t.Errorf("proxy WriteTimeout = %s, want 2m", s.proxy.WriteTimeout)
	}
}

// A client that never finishes its request headers (Slowloris) is
// disconnected after ReadHeaderTimeout.
func TestSlowHeadersAreCutOff(t *testing.T) {
	s := New(Config{Host: "127.0.0.1", ReadHeaderTimeout: 200 * time.Millisecond})
	s.SetHandlers(http.NotFoundHandler(), http.NotFoundHandler())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go s.proxy.Serve(ln) //nolint:errcheck
	defer s.proxy.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")); err != nil { // no final CRLF
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	start := time.Now()
	// The server closes the connection (possibly after a 408); reading to
	// EOF must finish well before the 5s deadline.
	r := bufio.NewReader(conn)
	for {
		if _, err := r.ReadByte(); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("connection still open after ReadHeaderTimeout")
			}
			break
		}
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("connection closed after %s, want about 200ms", d)
	}
}
