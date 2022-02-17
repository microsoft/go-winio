//go:build windows
// +build windows

package winio

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/Microsoft/go-winio/pkg/guid"
	"github.com/Microsoft/go-winio/pkg/sockets"
)

const (
	afHvSock = 34 // AF_HYPERV

	socketError = ^uintptr(0)
)

var (
	// https://docs.microsoft.com/en-us/virtualization/hyper-v-on-windows/user-guide/make-integration-service#vmid-wildcards

	// HVguidWildcard is the wildcard VmId for accepting connections from all partitions.
	HVguidWildcard = guid.GUID{} // 00000000-0000-0000-0000-000000000000

	// HVguidBroadcast is the wildcard VmId for broadcasting sends to all partitions
	HVguidBroadcast = guid.GUID{ //ffffffff-ffff-ffff-ffff-ffffffffffff
		Data1: 0xffffffff,
		Data2: 0xffff,
		Data3: 0xffff,
		Data4: [8]uint8{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}

	// HVGUIDLoopback is the Loopback VmId for accepting connections to the same partition as the connector.
	HVguidLoopback = guid.GUID{ // e0e16197-dd56-4a10-9195-5ee7a155a838
		Data1: 0xe0e16197,
		Data2: 0xdd56,
		Data3: 0x4a10,
		Data4: [8]uint8{0x91, 0x95, 0x5e, 0xe7, 0xa1, 0x55, 0xa8, 0x38},
	}

	// HVguidSiloHost is the address of a silo's host partition:
	// - The silo host of a hosted silo is the utility VM.
	// - The silo host of a silo on a physical host is the physical host.
	HVguidSiloHost = guid.GUID{ // 36bd0c5c-7276-4223-88ba-7d03b654c568
		Data1: 0x36bd0c5c,
		Data2: 0x7276,
		Data3: 0x4223,
		Data4: [8]byte{0x88, 0xba, 0x7d, 0x03, 0xb6, 0x54, 0xc5, 0x68},
	}

	// HVguidChildren is the wildcard VmId for accepting connections from the connector's child partitions.
	HVguidChildren = guid.GUID{ // 90db8b89-0d35-4f79-8ce9-49ea0ac8b7cd
		Data1: 0x90db8b89,
		Data2: 0xd35,
		Data3: 0x4f79,
		Data4: [8]uint8{0x8c, 0xe9, 0x49, 0xea, 0xa, 0xc8, 0xb7, 0xcd},
	}

	// HVguidParent is the wildcard VmId for accepting connections from the connector's parent partition.
	// Listening on this VmId accepts connection from:
	// - Inside silos: silo host partition.
	// - Inside hosted silo: host of the VM.
	// - Inside VM: VM host.
	// - Physical host: Not supported.
	HVguidParent = guid.GUID{ // a42e7cda-d03f-480c-9cc2-a4de20abb878
		Data1: 0xa42e7cda,
		Data2: 0xd03f,
		Data3: 0x480c,
		Data4: [8]uint8{0x9c, 0xc2, 0xa4, 0xde, 0x20, 0xab, 0xb8, 0x78},
	}

	// HVguidVSockServiceGUIDTemplate is the Service GUID used for the VSOCK protocol
	hvguidVSockServiceTemplate = guid.GUID{ // 00000000-facb-11e6-bd58-64006a7986d3
		Data2: 0xfacb,
		Data3: 0x11e6,
		Data4: [8]uint8{0xbd, 0x58, 0x64, 0x00, 0x6a, 0x79, 0x86, 0xd3},
	}
)

// An HvsockAddr is an address for a AF_HYPERV socket.
type HvsockAddr struct {
	VMID      guid.GUID
	ServiceID guid.GUID
}

type rawHvsockAddr struct {
	Family    uint16
	_         uint16
	VMID      guid.GUID
	ServiceID guid.GUID
}

var _ sockets.RawSockaddr = &rawHvsockAddr{}

// Network returns the address's network name, "hvsock".
func (addr *HvsockAddr) Network() string {
	return "hvsock"
}

func (addr *HvsockAddr) String() string {
	return fmt.Sprintf("%s:%s", &addr.VMID, &addr.ServiceID)
}

// VsockServiceID returns an hvsock service ID corresponding to the specified AF_VSOCK port.
func VsockServiceID(port uint32) guid.GUID {
	g := hvguidVSockServiceTemplate // make a copy
	g.Data1 = port
	return g
}

func (addr *HvsockAddr) raw() rawHvsockAddr {
	return rawHvsockAddr{
		Family:    afHvSock,
		VMID:      addr.VMID,
		ServiceID: addr.ServiceID,
	}
}

func (addr *HvsockAddr) fromRaw(raw *rawHvsockAddr) {
	addr.VMID = raw.VMID
	addr.ServiceID = raw.ServiceID
}

// Sockaddr interface allows use with `sockets.Bind()` and `.ConnectEx()`
func (r *rawHvsockAddr) Sockaddr() (ptr unsafe.Pointer, len int32, err error) {
	return unsafe.Pointer(r), int32(unsafe.Sizeof(rawHvsockAddr{})), nil
}

// Sockaddr interface allows use with `sockets.Bind()` and `.ConnectEx()`
func (r *rawHvsockAddr) FromBytes(b []byte) error {
	n := int(unsafe.Sizeof(rawHvsockAddr{}))

	if len(b) < n {
		return fmt.Errorf("got %d, want %d: %w", len(b), n, sockets.ErrBufferSize)
	}

	copy(unsafe.Slice((*byte)(unsafe.Pointer(r)), n), b[:n])
	if r.Family != afHvSock {
		return fmt.Errorf("got %d, want %d: %w", r.Family, afHvSock, sockets.ErrAddrFamily)
	}

	return nil
}

// HvsockListener is a socket listener for the AF_HYPERV address family.
type HvsockListener struct {
	sock *win32File
	addr HvsockAddr
}

// HvsockConn is a connected socket of the AF_HYPERV address family.
type HvsockConn struct {
	sock          *win32File
	local, remote HvsockAddr
}

func newHvSocket() (*win32File, error) {
	fd, err := syscall.Socket(afHvSock, syscall.SOCK_STREAM, 1)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}
	f, err := makeWin32File(fd)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}
	f.socket = true
	return f, nil
}

// ListenHvsock listens for connections on the specified hvsock address.
func ListenHvsock(addr *HvsockAddr) (_ *HvsockListener, err error) {
	l := &HvsockListener{addr: *addr}
	sock, err := newHvSocket()
	if err != nil {
		return nil, l.opErr("listen", err)
	}
	sa := addr.raw()
	err = sockets.Bind(windows.Handle(sock.handle), &sa)
	if err != nil {
		return nil, l.opErr("listen", os.NewSyscallError("socket", err))
	}
	err = syscall.Listen(sock.handle, 16)
	if err != nil {
		return nil, l.opErr("listen", os.NewSyscallError("listen", err))
	}
	return &HvsockListener{sock: sock, addr: *addr}, nil
}

func (l *HvsockListener) opErr(op string, err error) error {
	return &net.OpError{Op: op, Net: "hvsock", Addr: &l.addr, Err: err}
}

// Addr returns the listener's network address.
func (l *HvsockListener) Addr() net.Addr {
	return &l.addr
}

// Accept waits for the next connection and returns it.
func (l *HvsockListener) Accept() (_ net.Conn, err error) {
	sock, err := newHvSocket()
	if err != nil {
		return nil, l.opErr("accept", err)
	}
	defer func() {
		if sock != nil {
			sock.Close()
		}
	}()
	c, err := l.sock.prepareIo()
	if err != nil {
		return nil, l.opErr("accept", err)
	}
	defer l.sock.wg.Done()

	// AcceptEx, per documentation, requires an extra 16 bytes per address:
	// https://docs.microsoft.com/en-us/windows/win32/api/mswsock/nf-mswsock-acceptex
	const addrlen = uint32(16 + unsafe.Sizeof(rawHvsockAddr{}))
	var addrbuf [addrlen * 2]byte

	var bytes uint32
	err = syscall.AcceptEx(l.sock.handle, sock.handle, &addrbuf[0],
		0 /*rxdatalen*/, addrlen, addrlen, &bytes, &c.o)
	_, err = l.sock.asyncIo(c, nil /*deadlineHandler*/, bytes, err)
	if err != nil {
		return nil, l.opErr("accept", os.NewSyscallError("acceptex", err))
	}

	conn := &HvsockConn{
		sock: sock,
	}
	conn.local.fromRaw((*rawHvsockAddr)(unsafe.Pointer(&addrbuf[0])))
	conn.remote.fromRaw((*rawHvsockAddr)(unsafe.Pointer(&addrbuf[addrlen])))

	// initialize the accepted socket and update its properties with those of the listening socket
	if err = windows.Setsockopt(windows.Handle(sock.handle),
		windows.SOL_SOCKET, windows.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&l.sock.handle)), int32(unsafe.Sizeof(l.sock.handle))); err != nil {
		return nil, conn.opErr("accept", os.NewSyscallError("setsockopt", err))
	}

	// The local address returned in the AcceptEx buffer is the same as the Listener socket's
	// address. However, the service GUID reported by GetSockName is different from the Listeners
	// socket, and is sometimes the same as the local address of the socket that dialed the
	// address, with the service GUID.Data1 incremented, but othertimes is different.
	// todo: does the local address matter? is the listener's address or the actual address appropriate?

	// var ra rawHvsockAddr
	// err = sockets.GetSockName(windows.Handle(sock.handle), &ra)
	// if err != nil {
	// 	return nil, conn.opErr("accept", os.NewSyscallError("getsockname", err))
	// }
	// conn.local.fromRaw(&ra)

	sock = nil
	return conn, nil
}

// Close closes the listener, causing any pending Accept calls to fail.
func (l *HvsockListener) Close() error {
	return l.sock.Close()
}

type HvsockDialer struct {
	// Deadline is the time the Dial operation must connect before erroring.
	Deadline time.Time

	// Retries is the number of additional connects to try if the connection times out, is refused,
	// or the host is unreachable
	Retries uint

	// RetryWait is the time to wait after a connection error to retry
	RetryWait time.Duration

	rt *time.Timer // redial wait timer
}

func (d *HvsockDialer) Dial(addr *HvsockAddr) (*HvsockConn, error) {
	return d.DialContext(context.Background(), addr)
}

func (d *HvsockDialer) DialContext(ctx context.Context, addr *HvsockAddr) (conn *HvsockConn, err error) {
	op := "dial"
	// create the conn early to use opErr()
	conn = &HvsockConn{
		remote: *addr,
	}

	if !d.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, d.Deadline)
		defer cancel()
	}

	// preemptive timeout/cancellation check
	if err = ctx.Err(); err != nil {
		return nil, conn.opErr(op, err)
	}

	sock, err := newHvSocket()
	if err != nil {
		return nil, conn.opErr("dial", err)
	}
	defer func() {
		if sock != nil {
			sock.Close()
		}
	}()

	sa := addr.raw()
	err = sockets.Bind(windows.Handle(sock.handle), &sa)
	if err != nil {
		return nil, conn.opErr(op, os.NewSyscallError("bind", err))
	}

	c, err := sock.prepareIo()
	if err != nil {
		return nil, conn.opErr(op, err)
	}
	defer sock.wg.Done()
	var bytes uint32
	n := 1 + int(d.Retries)
	for i := 1; i <= n; i++ {
		err = sockets.ConnectEx(
			windows.Handle(sock.handle), &sa,
			nil /*sendBuf*/, 0, /*sendDataLen*/
			&bytes, (*windows.Overlapped)(unsafe.Pointer(&c.o)))
		// todo: create an asyncIO version that takes a context
		// could create a deadlineHandler triggered by context cancelation, but that seems inefficient ...
		_, err = sock.asyncIo(c, nil, bytes, err)
		if i < n && canRedial(err) {
			if err = d.redialWait(ctx); err != nil {
				break
			}
			continue
		}
		break
	}
	if err != nil {
		return nil, conn.opErr(op, os.NewSyscallError("connectex", err))
	}

	// update the connection properties, so shutdown can be used
	if err = windows.Setsockopt(windows.Handle(sock.handle),
		windows.SOL_SOCKET, windows.SO_UPDATE_CONNECT_CONTEXT,
		nil /*optvalue*/, 0 /*optlen*/); err != nil {
		return nil, conn.opErr(op, os.NewSyscallError("setsockopt", err))
	}

	// get the local name
	var sal rawHvsockAddr
	err = sockets.GetSockName(windows.Handle(sock.handle), &sal)
	if err != nil {
		return nil, conn.opErr(op, os.NewSyscallError("getsockname", err))
	}
	conn.local.fromRaw(&sal)

	// one last check for timeout, since asyncIO doesnt check the context
	if err = ctx.Err(); err != nil {
		return nil, conn.opErr(op, err)
	}

	conn.sock = sock
	sock = nil

	return conn, nil
}

func (d *HvsockDialer) redialWait(ctx context.Context) (err error) {
	if d.RetryWait == 0 {
		return nil
	}

	if d.rt == nil {
		d.rt = time.NewTimer(d.RetryWait)
	} else {
		// should already be stopped and drained
		d.rt.Reset(d.RetryWait)
	}

	select {
	case <-ctx.Done():
	case <-d.rt.C:
		return nil
	}

	// stop and drain the timer
	if !d.rt.Stop() {
		<-d.rt.C
	}
	return ctx.Err()
}

// assumes error is a plain, unwrapped syscall.Errno
func canRedial(err error) bool {
	switch err {
	case windows.WSAECONNREFUSED, windows.WSAENETUNREACH, windows.WSAETIMEDOUT,
		windows.ERROR_CONNECTION_REFUSED, windows.ERROR_CONNECTION_UNAVAIL:
		return true
	default:
		return false
	}
}

func (conn *HvsockConn) opErr(op string, err error) error {
	return &net.OpError{Op: op, Net: "hvsock", Source: &conn.local, Addr: &conn.remote, Err: err}
}

func (conn *HvsockConn) Read(b []byte) (int, error) {
	c, err := conn.sock.prepareIo()
	if err != nil {
		return 0, conn.opErr("read", err)
	}
	defer conn.sock.wg.Done()
	buf := syscall.WSABuf{Buf: &b[0], Len: uint32(len(b))}
	var flags, bytes uint32
	err = syscall.WSARecv(conn.sock.handle, &buf, 1, &bytes, &flags, &c.o, nil)
	n, err := conn.sock.asyncIo(c, &conn.sock.readDeadline, bytes, err)
	if err != nil {
		if _, ok := err.(syscall.Errno); ok {
			err = os.NewSyscallError("wsarecv", err)
		}
		return 0, conn.opErr("read", err)
	} else if n == 0 {
		err = io.EOF
	}
	return n, err
}

func (conn *HvsockConn) Write(b []byte) (int, error) {
	t := 0
	for len(b) != 0 {
		n, err := conn.write(b)
		if err != nil {
			return t + n, err
		}
		t += n
		b = b[n:]
	}
	return t, nil
}

func (conn *HvsockConn) write(b []byte) (int, error) {
	c, err := conn.sock.prepareIo()
	if err != nil {
		return 0, conn.opErr("write", err)
	}
	defer conn.sock.wg.Done()
	buf := syscall.WSABuf{Buf: &b[0], Len: uint32(len(b))}
	var bytes uint32
	err = syscall.WSASend(conn.sock.handle, &buf, 1, &bytes, 0, &c.o, nil)
	n, err := conn.sock.asyncIo(c, &conn.sock.writeDeadline, bytes, err)
	if err != nil {
		if _, ok := err.(syscall.Errno); ok {
			err = os.NewSyscallError("wsasend", err)
		}
		return 0, conn.opErr("write", err)
	}
	return n, err
}

// Close closes the socket connection, failing any pending read or write calls.
func (conn *HvsockConn) Close() error {
	return conn.sock.Close()
}

func (conn *HvsockConn) IsClosed() bool {
	return conn.sock.IsClosed()
}

// shutdown disables sending or receiving on a socket
func (conn *HvsockConn) shutdown(how int) error {
	if conn.IsClosed() {
		return ErrFileClosed
	}

	err := syscall.Shutdown(conn.sock.handle, how)
	if err != nil {
		return os.NewSyscallError("shutdown", err)
	}
	return nil
}

// CloseRead shuts down the read end of the socket, preventing future read operations.
func (conn *HvsockConn) CloseRead() error {
	err := conn.shutdown(syscall.SHUT_RD)
	if err != nil {
		return conn.opErr("closeread", err)
	}
	return nil
}

// CloseWrite shuts down the write end of the socket, preventing future write operations and
// notifying the other endpoint that no more data will be written.
func (conn *HvsockConn) CloseWrite() error {
	err := conn.shutdown(syscall.SHUT_WR)
	if err != nil {
		return conn.opErr("closewrite", err)
	}
	return nil
}

// LocalAddr returns the local address of the connection.
func (conn *HvsockConn) LocalAddr() net.Addr {
	return &conn.local
}

// RemoteAddr returns the remote address of the connection.
func (conn *HvsockConn) RemoteAddr() net.Addr {
	return &conn.remote
}

// SetDeadline implements the net.Conn SetDeadline method.
func (conn *HvsockConn) SetDeadline(t time.Time) error {
	conn.SetReadDeadline(t)
	conn.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline implements the net.Conn SetReadDeadline method.
func (conn *HvsockConn) SetReadDeadline(t time.Time) error {
	return conn.sock.SetReadDeadline(t)
}

// SetWriteDeadline implements the net.Conn SetWriteDeadline method.
func (conn *HvsockConn) SetWriteDeadline(t time.Time) error {
	return conn.sock.SetWriteDeadline(t)
}

// HvSockRegisterService registers the application defined by the guid and name with the
// Hyper-V Host's registry.
//
// See: https://docs.microsoft.com/en-us/virtualization/hyper-v-on-windows/user-guide/make-integration-service#register-a-new-application
func HvSockRegisterService(id guid.GUID, name string) error {
	k, exists, err := registry.CreateKey(registry.LOCAL_MACHINE, hvSocketRegKey(id), registry.WRITE)
	if err != nil {
		return fmt.Errorf("could not create registry for %s: %w", id.String(), err)
	}
	defer k.Close()

	if exists {
		return nil
	}
	if err = k.SetStringValue("ElementName", name); err != nil {
		return fmt.Errorf("could not set service name to %s: %w", name, err)
	}

	return nil
}

// HvSockUnregisterService deleted the registration defined by the guid from the Hyper-V Host's registry.
func HvSockUnegisterService(id guid.GUID) error {
	return registry.DeleteKey(registry.LOCAL_MACHINE, hvSocketRegKey(id))
}

func hvSocketRegKey(id guid.GUID) string {
	return `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Virtualization\GuestCommunicationServices\` + id.String()
}
