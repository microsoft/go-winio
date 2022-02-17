//go:build windows

package winio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/pkg/sockets"
	"golang.org/x/sys/windows"
)

// TODO: timeouts on listen

// var addr = &HvsockAddr{
// 	VMID:      HVguidLoopback,
// 	ServiceID: VsockServiceID(58),
// }

func randHvsockAddr() *HvsockAddr {
	p := uint32(rand.Int31())
	return &HvsockAddr{
		VMID:      HVguidLoopback,
		ServiceID: VsockServiceID(p),
	}

}

func serverListen(t *testing.T) (*HvsockListener, *HvsockAddr) {
	a := randHvsockAddr()
	l, err := ListenHvsock(a)
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Logf("could not close Hyper-V socket listener: %v", err)
		}
	})

	return l, a
}

func TestHvSockService(t *testing.T) {
	a := hvguidVSockServiceTemplate
	b := hvguidVSockServiceTemplate
	a.Data1 = 2016

	fmt.Println("a", a)
	fmt.Println("b", b)

}

func TestHvSockConstants(t *testing.T) {
	// not really constants ...
	tests := []struct {
		name string
		want string
		give guid.GUID
	}{
		{"wildcard", "00000000-0000-0000-0000-000000000000", HVguidWildcard},
		{"broadcast", "ffffffff-ffff-ffff-ffff-ffffffffffff", HVguidBroadcast},
		{"loopback", "e0e16197-dd56-4a10-9195-5ee7a155a838", HVguidLoopback},
		{"children", "90db8b89-0d35-4f79-8ce9-49ea0ac8b7cd", HVguidChildren},
		{"parent", "a42e7cda-d03f-480c-9cc2-a4de20abb878", HVguidParent},
		{"silohost", "36bd0c5c-7276-4223-88ba-7d03b654c568", HVguidSiloHost},
		{"vsock template", "00000000-facb-11e6-bd58-64006a7986d3", hvguidVSockServiceTemplate},
	}
	for _, tt := range tests {
		if tt.give.String() != tt.want {
			t.Errorf("%s give: %v; want: %s", tt.name, tt.give, tt.want)
		}
	}
}
func TestHvSockAddresses(t *testing.T) {
	errs := make(chan error)
	defer close(errs)

	l, addr := serverListen(t)
	var sv *HvsockConn
	go func() {
		ss, err := l.Accept()
		sv = ss.(*HvsockConn)
		if err != nil {
			errs <- fmt.Errorf("listener accept error: %w", err)
			return
		}
		errs <- nil
	}()

	cl, err := (&HvsockDialer{}).Dial(addr)
	if err != nil {
		<-errs // wait on the go routine before closing it
		t.Fatalf("could not dial: %s", err)
	}
	defer cl.Close()

	if err := <-errs; err != nil {
		t.Fatalf(err.Error())
	}
	defer sv.Close()

	la := (l.Addr()).(*HvsockAddr)
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
			{"listener", la, *addr},
			{"client local", cla, HvsockAddr{HVguidChildren, sra.ServiceID}},
			{"client remote", cra, *addr},
			{"server local", sla, HvsockAddr{HVguidChildren, addr.ServiceID}},
			{"server remote", sra, HvsockAddr{HVguidLoopback, cla.ServiceID}},
		}
		for _, tt := range tests {
			if *tt.give != tt.want {
				t.Errorf("%s address give: %v; want: %v", tt.name, tt.give, tt.want)
			}
		}
	})

	t.Run("OSinfo", func(t *testing.T) {
		ra := rawHvsockAddr{}
		sa := HvsockAddr{}

		localTests := []struct {
			name     string
			giveSock *win32File
			wantAddr HvsockAddr
		}{
			{"listener", l.sock, *addr},
			{"client", cl.sock, HvsockAddr{HVguidChildren, cla.ServiceID}},
			// The server sockets local address seems arbitrary, so skip this test
			// see comment in `(*HvsockListener) Accept()` for more info
			// {"server", sv.sock, _sla},
		}
		for _, tt := range localTests {
			sockets.GetSockName(windows.Handle(tt.giveSock.handle), &ra)
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
			sockets.GetPeerName(windows.Handle(tt.giveConn.sock.handle), &ra)
			sa.fromRaw(&ra)
			if sa != tt.giveConn.remote {
				t.Errorf("%s remote addr give: %v; want: %v", tt.name, sa, tt.giveConn.remote)
			}
		}
	})
}

func TestHvSockReadWrite(t *testing.T) {
	svch := make(chan error)
	defer close(svch)
	clch := make(chan error)
	defer close(clch)

	l, addr := serverListen(t)

	tests := []struct {
		req, rsp string
	}{
		{"hello ", "world!"},
		{"ping", "pong"},
	}

	go func() {
		c, err := l.Accept()
		if err != nil {
			svch <- fmt.Errorf("listener accept error: %w", err)
			return
		}
		defer c.Close()

		b := make([]byte, 64)
		for _, tt := range tests {
			n, err := c.Read(b)
			if err != nil {
				svch <- fmt.Errorf("server rx error: %w", err)
				return
			}

			r := string(b[:n])

			if r != tt.req {
				svch <- fmt.Errorf("server rx error, actual %q, expected %q", b[:n], tt.req)
				return
			}

			if _, err = c.Write([]byte(tt.rsp)); err != nil {
				svch <- fmt.Errorf("server tx error, could not send %q: %w", tt.rsp, err)
				return
			}
		}
		n, err := c.Read(b)
		if err != io.EOF && n != 0 {
			svch <- fmt.Errorf("expected 0 bytes and EOF, actual %d, %v", n, err)
			return
		}

		svch <- nil
	}()

	var cl *HvsockConn
	go func() {
		var err error
		cl, err = (&HvsockDialer{}).Dial(addr)
		if err != nil {
			clch <- fmt.Errorf("client  dial error: %w", err)
			return
		}
		defer cl.Close()

		b := make([]byte, 64)
		for _, tt := range tests {
			if _, err := cl.Write([]byte(tt.req)); err != nil {
				clch <- fmt.Errorf("client tx error, could not send %q: %w", tt.req, err)
				return
			}

			n, err := cl.Read(b)
			if err != nil {
				clch <- fmt.Errorf("client rx error: %w", err)
				return
			}

			r := string(b[:n])
			if r != tt.rsp {
				clch <- fmt.Errorf("client  rx error, actual %q, expected %q", b[:n], tt.rsp)
				return
			}
		}

		cl.CloseWrite()
		clch <- nil
	}()

	var err error
	tr := time.NewTimer(time.Minute)
	defer tr.Stop()

	select {
	case <-tr.C:
		err = fmt.Errorf("test timed out")
	case err = <-svch:
	case err = <-clch:
	}
	if err != nil {
		t.Error(err.Error())
		l.Close()
		cl.Close()
	}

	// grab the other error too
	select {
	case err = <-svch:
	case err = <-clch:
	}
	if err != nil {
		t.Errorf(err.Error())
	}
}

func TestHvSockReadTooSmall(t *testing.T) {
	errs := make(chan error)
	defer close(errs)

	s := "this is a really long string that hopefully takes up more than 16 bytes ..."
	l, addr := serverListen(t)

	go func() {
		c, err := l.Accept()
		if err != nil {
			errs <- fmt.Errorf("listener accept error: %w", err)
			return
		}
		defer c.Close()

		b := make([]byte, 16)
		ss := ""
		for {
			n, err := c.Read(b)
			if err == io.EOF {
				break
			} else if err != nil {
				errs <- fmt.Errorf("server rx error: %w", err)
				return
			}
			ss += string(b[:n])
		}

		if ss != s {
			errs <- fmt.Errorf("got wrong string: %q", ss)
		}
		errs <- nil
	}()

	var cl *HvsockConn
	go func() {
		var err error
		cl, err = (&HvsockDialer{}).Dial(addr)
		if err != nil {
			errs <- fmt.Errorf("client  dial error: %w", err)
			return
		}
		defer cl.Close()

		if _, err := cl.Write([]byte(s)); err != nil {
			errs <- fmt.Errorf("client tx error, could not send: %w", err)
			return
		}
		errs <- nil
	}()

	var err error
	tr := time.NewTimer(time.Minute)
	defer tr.Stop()

	select {
	case <-tr.C:
		err = fmt.Errorf("test timed out")
	case err = <-errs:
	}
	if err != nil {
		t.Error(err.Error())
		l.Close()
		cl.Close()
	}

	// grab the other error too
	if err := <-errs; err != nil {
		t.Errorf(err.Error())
	}
}

func TestHvSockCloseReadWriteListener(t *testing.T) {
	errs := make(chan error)
	defer close(errs)
	syn := make(chan struct{})
	defer close(syn)
	defer func() {
		// make sure the go routine ends before closing the channels
		if err := <-errs; err != nil {
			t.Error(err.Error())
		}
	}()

	l, addr := serverListen(t)

	go func() {
		c, err := l.Accept()
		if err != nil {
			errs <- fmt.Errorf("listener accept error: %w", err)
			return
		}
		defer c.Close()

		//
		// test CloseWrite()
		//
		_, err = c.Write([]byte("test"))
		if err != nil {
			errs <- fmt.Errorf("server tx error: %w", err)
			return
		}

		cw := c.(sockets.CloseWriter)
		if err = cw.CloseWrite(); err != nil {
			errs <- fmt.Errorf("server close write: %w", err)
			return
		}

		_, err = c.Write([]byte("test"))
		if !errors.Is(err, windows.WSAESHUTDOWN) {
			errs <- fmt.Errorf("server did not shutdown writes: %w", err)
			return
		}

		// safe to call multiple times
		if err = cw.CloseWrite(); err != nil {
			errs <- fmt.Errorf("server second close write: %w", err)
			return
		}

		//
		// test CloseRead()
		//
		b := make([]byte, 256)
		n, err := c.Read(b)
		if err != nil {
			errs <- fmt.Errorf("server read: %w", err)
			return
		}
		if string(b[:n]) != "test" {
			errs <- fmt.Errorf("expected %q, actual %q", "test", b[:n])
			return
		}

		cr := c.(sockets.CloseReader)
		if err = cr.CloseRead(); err != nil {
			errs <- fmt.Errorf("server close read: %w", err)
			return
		}
		syn <- struct{}{}
		// signal the client to send more info
		// if it was sent before, the read would succeed if the data was buffered prior
		_, err = c.Read(b)
		if !errors.Is(err, windows.WSAESHUTDOWN) {
			errs <- fmt.Errorf("server did not shutdown reads: %w", err)
			return
		}

		// safe to call multiple times
		if err = cr.CloseRead(); err != nil {
			errs <- fmt.Errorf("server second close read: %w", err)
			return
		}

		c.Close()
		if err = cw.CloseWrite(); !errors.Is(err, ErrFileClosed) {
			errs <- fmt.Errorf("client close write did not return `ErrFileClosed`: %w", err)
			return
		}

		if err = cr.CloseRead(); !errors.Is(err, ErrFileClosed) {
			errs <- fmt.Errorf("client close read did not return `ErrFileClosed`: %w", err)
			return
		}

		errs <- nil
	}()

	cl, err := (&HvsockDialer{}).Dial(addr)
	if err != nil {
		t.Fatalf("could not dial: %s", err)
	}
	defer cl.Close()

	b := make([]byte, 256)
	n, err := cl.Read(b)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(b[:n]) != "test" {
		t.Fatalf("expected %q, actual %q", "test", b[:n])
	}

	n, err = cl.Read(b)
	if n != 0 && err != io.EOF {
		t.Fatalf("client did not get EOF: %v", err)
	}

	_, err = cl.Write([]byte("test"))
	if err != nil {
		t.Fatalf("client write: %v", err)
	}
	<-syn
	// this should succeed
	_, err = cl.Write([]byte("test2"))
	if err != nil {
		t.Fatalf("client write: %v", err)
	}

}

func TestHvSockCloseReadWriteDial(t *testing.T) {
	errs := make(chan error)
	defer close(errs)
	syn := make(chan struct{})
	defer close(syn)

	defer func() {
		// make sure the go routine ends before closing the channels
		if err := <-errs; err != nil {
			t.Errorf(err.Error())
		}
	}()

	l, addr := serverListen(t)

	go func() {
		c, err := l.Accept()
		if err != nil {
			errs <- fmt.Errorf("listener accept error: %w", err)
			return
		}
		defer c.Close()

		b := make([]byte, 256)
		n, err := c.Read(b)
		if err != nil {
			errs <- fmt.Errorf("server read: %w", err)
			return
		}
		if string(b[:n]) != "test" {
			errs <- fmt.Errorf("expected %q, actual %q", "test", b[:n])
			return
		}

		n, err = c.Read(b)
		if n != 0 && err != io.EOF {
			errs <- fmt.Errorf("server did not get EOF: %w", err)
			return
		}

		_, err = c.Write([]byte("test"))
		if err != nil {
			errs <- fmt.Errorf("server tx error: %w", err)
			return
		}
		<-syn
		_, err = c.Write([]byte("test"))
		if err != nil {
			errs <- fmt.Errorf("server tx error: %w", err)
			return
		}

		c.Close()
		errs <- nil
	}()

	cl, err := (&HvsockDialer{}).Dial(addr)
	if err != nil {
		t.Fatalf("could not dial: %s", err)
	}
	defer cl.Close()

	//
	// test CloseWrite()
	//
	_, err = cl.Write([]byte("test"))
	if err != nil {
		t.Fatalf("client write: %v", err)
	}

	if err = cl.CloseWrite(); err != nil {
		t.Fatalf("client close write: %v", err)
	}

	_, err = cl.Write([]byte("test"))
	if !errors.Is(err, windows.WSAESHUTDOWN) {
		t.Fatalf("client did not shutdown writes: %v", err)
	}

	// safe to call multiple times
	if err = cl.CloseWrite(); err != nil {
		t.Fatalf("client second close write: %v", err)
	}

	//
	// test CloseRead()
	//
	b := make([]byte, 256)
	n, err := cl.Read(b)
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(b[:n]) != "test" {
		t.Fatalf("expected %q, actual %q", "test", b[:n])
	}

	if err = cl.CloseRead(); err != nil {
		t.Fatalf("client close read: %v", err)
	}

	syn <- struct{}{}
	// signal the client to send more info
	// if it was sent before, the read would succeed if the data was buffered prior
	_, err = cl.Read(b)
	if !errors.Is(err, windows.WSAESHUTDOWN) {
		t.Fatalf("client did not shutdown reads: %v", err)
	}

	// safe to call multiple times
	if err = cl.CloseRead(); err != nil {
		t.Fatalf("client second close write: %v", err)
	}

	l.Close()
	cl.Close()

	if err = cl.CloseWrite(); !errors.Is(err, ErrFileClosed) {
		t.Fatalf("client close write did not return `ErrFileClosed`: %v", err)
	}

	if err = cl.CloseRead(); !errors.Is(err, ErrFileClosed) {
		t.Fatalf("client close read did not return `ErrFileClosed`: %v", err)
	}
}

func TestHvSockDialNoTimeout(t *testing.T) {
	errs := make(chan error)
	defer close(errs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		addr := randHvsockAddr()
		cl, err := (&HvsockDialer{}).DialContext(ctx, addr)
		if err != nil {
			errs <- err
			return
		}
		defer cl.Close()
		errs <- errors.New("should not have gotten here")
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, windows.WSAECONNREFUSED) {
			t.Fatalf("expected connection refused error, actual: %v", err)
		}
	// connections usually take about ~500µs
	case <-time.After(2 * time.Millisecond):
		t.Fatalf("dial did not time out")
	}
}

func TestHvSockDialDeadline(t *testing.T) {
	d := &HvsockDialer{}
	d.Deadline = time.Now().Add(50 * time.Microsecond)
	d.Retries = 1
	// we need the wait time to be long enough for the deadline goroutine to run first and signal
	// timeout
	d.RetryWait = 100 * time.Millisecond
	addr := randHvsockAddr()
	cl, err := d.Dial(addr)
	if err == nil {
		cl.Close()
		t.Fatalf("dial should not have finished")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dial did not exceed deadline: %v", err)
	}
}

func TestHvSockDialContext(t *testing.T) {
	errs := make(chan error)
	defer close(errs)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Microsecond, cancel)

	d := &HvsockDialer{}
	d.Retries = 1
	d.RetryWait = 100 * time.Millisecond
	addr := randHvsockAddr()
	cl, err := d.DialContext(ctx, addr)
	if err == nil {
		cl.Close()
		t.Fatalf("dial should not have finished")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("dial was not canceled: %v", err)
	}
}

func TestHvSockAcceptClose(t *testing.T) {
	l, _ := serverListen(t)
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.Close()
	}()

	c, err := l.Accept()
	if err == nil {
		c.Close()
		t.Fatal("listener should not have accepted anything")
	} else if !errors.Is(err, ErrFileClosed) {
		t.Fatalf("expected %v, actual %v", ErrFileClosed, err)
	}
}
