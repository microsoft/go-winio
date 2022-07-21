//go:build windows

package winio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/Microsoft/go-winio/internal/socket"
	"github.com/Microsoft/go-winio/pkg/guid"
)

// TODO: timeouts on listen

const testStr = "test"

func randHvsockAddr() *HvsockAddr {
	p := uint32(rand.Int31())
	return &HvsockAddr{
		VMID:      HvsockGUIDLoopback(),
		ServiceID: VsockServiceID(p),
	}
}

func serverListen(u testUtil) (*HvsockListener, *HvsockAddr) {
	a := randHvsockAddr()
	l, err := ListenHvsock(a)
	u.Must(err, "could not listen")
	u.T.Cleanup(func() {
		if l != nil {
			u.Must(l.Close(), "Hyper-V socket listener close")
		}
	})

	return l, a
}

func clientServer(u testUtil) (cl, sv *HvsockConn, _ *HvsockAddr) {
	l, addr := serverListen(u)
	ch := make(chan struct{})
	go func() {
		defer close(ch)

		conn, err := l.Accept()
		u.Must(err, "listener accept")
		sv = conn.(*HvsockConn)
		u.T.Cleanup(func() {
			if sv != nil {
				u.Must(sv.Close(), "server close")
			}
		})
		u.Must(l.Close())
		l = nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cl, err := Dial(ctx, addr)
	u.Must(err, "could not dial")
	u.T.Cleanup(func() {
		if cl != nil {
			u.Must(cl.Close(), "client close")
		}
	})

	u.Wait(ch, time.Second)
	u.Check()
	return cl, sv, addr
}

func TestHvSockConstants(t *testing.T) {
	tests := []struct {
		name string
		want string
		give guid.GUID
	}{
		{"wildcard", "00000000-0000-0000-0000-000000000000", HvsockGUIDWildcard()},
		{"broadcast", "ffffffff-ffff-ffff-ffff-ffffffffffff", HvsockGUIDBroadcast()},
		{"loopback", "e0e16197-dd56-4a10-9195-5ee7a155a838", HvsockGUIDLoopback()},
		{"children", "90db8b89-0d35-4f79-8ce9-49ea0ac8b7cd", HvsockGUIDChildren()},
		{"parent", "a42e7cda-d03f-480c-9cc2-a4de20abb878", HvsockGUIDParent()},
		{"silohost", "36bd0c5c-7276-4223-88ba-7d03b654c568", HvsockGUIDSiloHost()},
		{"vsock template", "00000000-facb-11e6-bd58-64006a7986d3", hvsockVsockServiceTemplate()},
	}
	for _, tt := range tests {
		if tt.give.String() != tt.want {
			t.Errorf("%s give: %v; want: %s", tt.name, tt.give, tt.want)
		}
	}
}

func TestHvSockListenerAddresses(t *testing.T) {
	u := newUtil(t)
	l, addr := serverListen(u)

	la := (l.Addr()).(*HvsockAddr)
	u.Assert(*la == *addr, fmt.Sprintf("give: %v; want: %v", la, addr))

	ra := rawHvsockAddr{}
	sa := HvsockAddr{}
	u.Must(socket.GetSockName(windows.Handle(l.sock.handle), &ra))
	sa.fromRaw(&ra)
	u.Assert(sa == *addr, fmt.Sprintf("listener local addr give: %v; want: %v", sa, addr))
}

func TestHvSockAddresses(t *testing.T) {
	u := newUtil(t)
	cl, sv, addr := clientServer(u)

	sra := (sv.RemoteAddr()).(*HvsockAddr)
	sla := (sv.LocalAddr()).(*HvsockAddr)
	cra := (cl.RemoteAddr()).(*HvsockAddr)
	cla := (cl.LocalAddr()).(*HvsockAddr)

	t.Run("Info", func(t *testing.T) {
		tests := []struct {
			name string
			give *HvsockAddr
			want HvsockAddr
		}{
			{"client local", cla, HvsockAddr{HvsockGUIDChildren(), sra.ServiceID}},
			{"client remote", cra, *addr},
			{"server local", sla, HvsockAddr{HvsockGUIDChildren(), addr.ServiceID}},
			{"server remote", sra, HvsockAddr{HvsockGUIDLoopback(), cla.ServiceID}},
		}
		for _, tt := range tests {
			if *tt.give != tt.want {
				t.Errorf("%s address give: %v; want: %v", tt.name, tt.give, tt.want)
			}
		}
	})

	t.Run("OSinfo", func(t *testing.T) {
		u := newUtil(t)
		ra := rawHvsockAddr{}
		sa := HvsockAddr{}

		localTests := []struct {
			name     string
			giveSock *win32File
			wantAddr HvsockAddr
		}{
			{"client", cl.sock, HvsockAddr{HvsockGUIDChildren(), cla.ServiceID}},
			// The server sockets local address seems arbitrary, so skip this test
			// see comment in `(*HvsockListener) Accept()` for more info
			// {"server", sv.sock, _sla},
		}
		for _, tt := range localTests {
			u.Must(socket.GetSockName(windows.Handle(tt.giveSock.handle), &ra))
			sa.fromRaw(&ra)
			if sa != tt.wantAddr {
				t.Errorf("%s local addr give: %v; want: %v", tt.name, sa, tt.wantAddr)
			}
		}

		remoteTests := []struct {
			name     string
			giveConn *HvsockConn
		}{
			{"client", cl},
			{"server", sv},
		}
		for _, tt := range remoteTests {
			u.Must(socket.GetPeerName(windows.Handle(tt.giveConn.sock.handle), &ra))
			sa.fromRaw(&ra)
			if sa != tt.giveConn.remote {
				t.Errorf("%s remote addr give: %v; want: %v", tt.name, sa, tt.giveConn.remote)
			}
		}
	})
}

func TestHvSockReadWrite(t *testing.T) {
	u := newUtil(t)
	l, addr := serverListen(u)
	tests := []struct {
		req, rsp string
	}{
		{"hello ", "world!"},
		{"ping", "pong"},
	}

	// a sync.WaitGroup doesnt offer a channel to use in a select with a timeout
	// could use an errgroup.Group, but for now dual channels work fine
	svCh := make(chan struct{})
	go func() {
		defer close(svCh)
		c, err := l.Accept()
		u.Must(err, "listener accept")
		defer c.Close()

		b := make([]byte, 64)
		for _, tt := range tests {
			n, err := c.Read(b)
			u.Must(err, "server rx")

			r := string(b[:n])
			u.Assert(r == tt.req, fmt.Sprintf("server rx error: got %q; wanted %q", r, tt.req))

			_, err = c.Write([]byte(tt.rsp))
			u.Must(err, "server tx error, could not send "+tt.rsp)
		}
		n, err := c.Read(b)
		u.Assert(n == 0, "server did not get EOF")
		u.Is(err, io.EOF, "server did not get EOF")
	}()

	clCh := make(chan struct{})
	go func() {
		defer close(clCh)
		cl, err := Dial(context.Background(), addr)
		u.Must(err, "client  dial")
		defer cl.Close()

		b := make([]byte, 64)
		for _, tt := range tests {
			_, err := cl.Write([]byte(tt.req))
			u.Must(err, "client tx error, could not send "+tt.req)

			n, err := cl.Read(b)
			u.Must(err, "client rx")

			r := string(b[:n])
			u.Assert(r == tt.rsp, fmt.Sprintf("client rx error: got %q; wanted %q", b[:n], tt.rsp))
		}

		u.Must(cl.CloseWrite())
	}()

	u.Wait(svCh, 15*time.Second, "server")
	u.Wait(clCh, 15*time.Second, "client")
}

func TestHvSockReadTooSmall(t *testing.T) {
	u := newUtil(t)
	s := "this is a really long string that hopefully takes up more than 16 bytes ..."
	l, addr := serverListen(u)

	svCh := make(chan struct{})
	go func() {
		defer close(svCh)
		c, err := l.Accept()
		u.Must(err, "listener accept")
		defer c.Close()

		b := make([]byte, 16)
		ss := ""
		for {
			n, err := c.Read(b)
			if errors.Is(err, io.EOF) {
				break
			}
			u.Must(err, "server rx")
			ss += string(b[:n])
		}

		u.Assert(ss == s, fmt.Sprintf("got %q, wanted: %q", ss, s))
	}()

	clCh := make(chan struct{})
	go func() {
		defer close(clCh)
		cl, err := Dial(context.Background(), addr)
		u.Must(err, "client  dial")
		defer cl.Close()

		_, err = cl.Write([]byte(s))
		u.Must(err, "client tx error, could not send")
	}()

	u.Wait(svCh, 15*time.Second, "server")
	u.Wait(clCh, 15*time.Second, "client")
}

func TestHvSockCloseReadWriteListener(t *testing.T) {
	u := newUtil(t)
	l, addr := serverListen(u)

	ch := make(chan struct{})
	go func() {
		defer close(ch)
		c, err := l.Accept()
		u.Must(err, "listener accept")
		defer c.Close()

		hv := c.(*HvsockConn)
		//
		// test CloseWrite()
		//
		_, err = c.Write([]byte(testStr))
		u.Must(err, "server tx")

		u.Must(hv.CloseWrite(), "server close write")

		_, err = c.Write([]byte(testStr))
		u.Is(err, windows.WSAESHUTDOWN, "server did not shutdown writes")

		// safe to call multiple times
		u.Must(hv.CloseWrite(), "server second close write")

		//
		// test CloseRead()
		//
		b := make([]byte, 256)
		n, err := c.Read(b)
		u.Must(err, "server read")
		u.Assert(string(b[:n]) == testStr, fmt.Sprintf("server got %q; wanted %q", b[:n], testStr))

		u.Must(hv.CloseRead(), "server close read")

		ch <- struct{}{}

		// signal the client to send more info
		// if it was sent before, the read would succeed if the data was buffered prior
		_, err = c.Read(b)
		u.Is(err, windows.WSAESHUTDOWN, "server did not shutdown reads")

		// safe to call multiple times
		u.Must(hv.CloseRead(), "server second close read")

		c.Close()
		u.Is(hv.CloseWrite(), socket.ErrSocketClosed, "client close write")
		u.Is(hv.CloseRead(), socket.ErrSocketClosed, "client close read")
	}()

	cl, err := Dial(context.Background(), addr)
	u.Must(err, "could not dial")
	defer cl.Close()

	b := make([]byte, 256)
	n, err := cl.Read(b)
	u.Must(err, "client read")
	u.Assert(string(b[:n]) == testStr, fmt.Sprintf("client got %q; wanted %q", b[:n], testStr))

	n, err = cl.Read(b)
	u.Assert(n == 0, "client did not get EOF")
	u.Is(err, io.EOF, "client did not get EOF")

	_, err = cl.Write([]byte(testStr))
	u.Must(err, "client write")

	u.Wait(ch, time.Second)
	u.Check()

	// this should succeed
	_, err = cl.Write([]byte("test2"))
	u.Must(err, "client write")
}

func TestHvSockCloseReadWriteDial(t *testing.T) {
	u := newUtil(t)
	l, addr := serverListen(u)

	ch := make(chan struct{})
	go func() {
		defer close(ch)
		c, err := l.Accept()
		u.Must(err, "listener accept")
		defer c.Close()

		b := make([]byte, 256)
		n, err := c.Read(b)
		u.Must(err, "server read")
		u.Assert(string(b[:n]) == testStr, fmt.Sprintf("server got %q; wanted %q", b[:n], testStr))

		n, err = c.Read(b)
		u.Assert(n == 0, "server did not get EOF")
		u.Is(err, io.EOF, "server did not get EOF")

		_, err = c.Write([]byte(testStr))
		u.Must(err, "server tx")

		ch <- struct{}{}

		_, err = c.Write([]byte(testStr))
		u.Must(err, "server tx")

		c.Close()
	}()

	cl, err := Dial(context.Background(), addr)
	u.Must(err, "could not dial")
	defer cl.Close()

	//
	// test CloseWrite()
	//
	_, err = cl.Write([]byte(testStr))
	u.Must(err, "client write")
	u.Must(cl.CloseWrite(), "client close write")

	_, err = cl.Write([]byte(testStr))
	u.Is(err, windows.WSAESHUTDOWN, "client did not shutdown writes")

	// safe to call multiple times
	u.Must(cl.CloseWrite(), "client second close write")

	//
	// test CloseRead()
	//
	b := make([]byte, 256)
	n, err := cl.Read(b)
	u.Must(err, "client read")
	u.Assert(string(b[:n]) == testStr, fmt.Sprintf("client got %q; wanted %q", b[:n], testStr))
	u.Must(cl.CloseRead(), "client close read")

	u.Wait(ch, time.Millisecond)
	u.Check()

	// signal the client to send more info
	// if it was sent before, the read would succeed if the data was buffered prior
	_, err = cl.Read(b)
	u.Is(err, windows.WSAESHUTDOWN, "client did not shutdown reads")

	// safe to call multiple times
	u.Must(cl.CloseRead(), "client second close write")

	l.Close()
	cl.Close()

	wantErr := socket.ErrSocketClosed
	u.Is(cl.CloseWrite(), wantErr, "client close write")
	u.Is(cl.CloseRead(), wantErr, "client close read")
}

func TestHvSockDialNoTimeout(t *testing.T) {
	u := newUtil(t)
	ch := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer close(ch)

		addr := randHvsockAddr()
		cl, err := Dial(ctx, addr)
		if err == nil {
			cl.Close()
		}
		u.Is(err, windows.WSAECONNREFUSED)
	}()

	// connections usually take about ~500µs
	u.Wait(ch, 2*time.Millisecond, "dial did not time out")
}

func TestHvSockDialDeadline(t *testing.T) {
	u := newUtil(t)
	d := &HvsockDialer{}
	d.Deadline = time.Now().Add(50 * time.Microsecond)
	d.Retries = 1
	// we need the wait time to be long enough for the deadline goroutine to run first and signal
	// timeout
	d.RetryWait = 100 * time.Millisecond
	addr := randHvsockAddr()
	cl, err := d.Dial(context.Background(), addr)
	if err == nil {
		cl.Close()
		t.Fatalf("dial should not have finished")
	}
	u.Is(err, context.DeadlineExceeded, "dial did not exceed deadline")
}

func TestHvSockDialContext(t *testing.T) {
	u := newUtil(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Microsecond, cancel)

	d := &HvsockDialer{}
	d.Retries = 1
	d.RetryWait = 100 * time.Millisecond
	addr := randHvsockAddr()
	cl, err := d.Dial(ctx, addr)
	if err == nil {
		cl.Close()
		t.Fatalf("dial should not have finished")
	}
	u.Is(err, context.Canceled, "dial was not canceled")
}

func TestHvSockAcceptClose(t *testing.T) {
	u := newUtil(t)
	l, _ := serverListen(u)
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.Close()
	}()

	c, err := l.Accept()
	if err == nil {
		c.Close()
		t.Fatal("listener should not have accepted anything")
	}
	u.Is(err, ErrFileClosed)
}

func FuzzRxTx(f *testing.F) {
	for _, b := range [][]byte{
		[]byte("hello?"),
		[]byte("This is a really long string that should be a good example of the really long " +
			"payloads that may be sent over hvsockets when really long inputs are being used, tautologically. " +
			"That means that we will have to test with really long input sequences, which means that " +
			"we need to include really long byte sequences or strings in our testing so that we know that " +
			"the sockets can deal with really long inputs. Look at this key mashing: " +
			"sdflhsdfgkjdhskljjsad;kljfasd;lfkjsadl ;fasdjfopiwej09q34iur092\"i4o[piwajfliasdkf-012ior]-" +
			"01oi3;'lSD<Fplkasdjgoisaefjoiasdlj\"hgfoaisdkf';laksdjdf[poaiseefk-0923i4roi3qwjrf9" +
			"08sEJKEFOLIsaejf[09saEJFLKSADjf;lkasdjf;kljaslddhgaskghk"),
		{0x5c, 0xbd, 0xb5, 0xe7, 0x6b, 0xcb, 0xe7, 0x23, 0xff, 0x7a, 0x19, 0x77, 0x2c, 0xca, 0xab, 0x3b},
	} {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, a []byte) {
		if string(a) == "" {
			t.Skip("skipping empty string")
		}
		t.Logf("testing %q (%d)", a, len(a))
		u := newUtil(t)
		cl, sv, _ := clientServer(u)

		svCh := make(chan struct{})
		go func() {
			defer close(svCh)

			n, err := cl.Write(a)
			u.Must(err, "client write")
			u.Assert(n == len(a), "client did not send full message")
			t.Log("client sent")

			b := make([]byte, len(a)+5) // a little extra to make sure nothing else is sent
			n, err = cl.Read(b)
			u.Must(err, "cl read")
			u.Assert(n == len(a), "client did not read full message")
			bn := b[:n]
			u.Assert(string(a) == string(bn), fmt.Sprintf("payload mismatch %q != %q", a, bn))
			t.Log("client received")
		}()

		clCh := make(chan struct{})
		go func() {
			defer close(clCh)

			b := make([]byte, len(a)+5) // a little extra to make sure nothing else is sent
			n, err := sv.Read(b)
			u.Must(err, "server read")
			u.Assert(n == len(a), "server did not read full message")
			bn := b[:n]
			u.Assert(string(a) == string(bn), fmt.Sprintf("payload mismatch %q != %q", a, bn))
			t.Log("server received")

			n, err = sv.Write(bn)
			u.Must(err, "server write")
			u.Assert(n == len(bn), "server did not send full message")
			t.Log("server sent")
		}()
		u.Wait(svCh, 250*time.Millisecond)
		u.Wait(clCh, 250*time.Millisecond)
	})
}

//
// helpers
//

type testUtil struct {
	T testing.TB
}

func newUtil(t testing.TB) testUtil {
	return testUtil{
		T: t,
	}
}

// checks stops execution if testing failed in another go-routine
func (u testUtil) Check() {
	if u.T.Failed() {
		u.T.FailNow()
	}
}

func (u testUtil) Assert(b bool, msgs ...string) {
	if b {
		return
	}
	u.T.Helper()
	u.T.Fatalf(_msgJoin(msgs, "failed assertion"))
}

func (u testUtil) Is(err, target error, msgs ...string) {
	if errors.Is(err, target) {
		return
	}
	u.T.Helper()
	u.T.Fatalf(_msgJoin(msgs, "got error %q; wanted %q"), err, target)
}

func (u testUtil) Must(err error, msgs ...string) {
	if err == nil {
		return
	}
	u.T.Helper()
	u.T.Fatalf(_msgJoin(msgs, "%v"), err)
}

func (u testUtil) Wait(ch <-chan struct{}, d time.Duration, msgs ...string) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
		u.T.Helper()
		u.T.Fatalf(_msgJoin(msgs, "timed out after %v"), d)
	}
}

func _msgJoin(pre []string, s string) string {
	return strings.Join(append(pre, s), ": ")
}
