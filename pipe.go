package winio

import (
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"
)

//sys connectNamedPipe(pipe syscall.Handle, o *syscall.Overlapped) (err error) = ConnectNamedPipe
//sys createNamedPipe(name string, flags uint32, pipeMode uint32, maxInstances uint32, outSize uint32, inSize uint32, defaultTimeout uint32, sa *syscall.SecurityAttributes) (handle syscall.Handle, err error)  [failretval==syscall.InvalidHandle] = CreateNamedPipeW
//sys createFile(name string, access uint32, mode uint32, sa *syscall.SecurityAttributes, createmode uint32, attrs uint32, templatefile syscall.Handle) (handle syscall.Handle, err error) [failretval==syscall.InvalidHandle] = CreateFileW
//sys waitNamedPipe(name string, timeout uint32) (err error) = WaitNamedPipeW
//sys convertStringSecurityDescriptorToSecurityDescriptor(str string, revision uint32, sd *uintptr, size *uint32) (err error) = advapi32.ConvertStringSecurityDescriptorToSecurityDescriptorW
//sys localFree(mem uintptr) = LocalFree

const (
	cERROR_PIPE_BUSY      = syscall.Errno(231)
	cERROR_PIPE_CONNECTED = syscall.Errno(535)
	cERROR_SEM_TIMEOUT    = syscall.Errno(121)

	pipeFlagAccessDuplex  = 0x3
	pipeFlagFirstInstance = 0x80000

	pipeModeRejectRemoteClients = 0x8

	pipeUnlimitedInstances = 255
)

type win32Pipe struct {
	*win32File
	path string
}

type pipeAddress string

func (f *win32Pipe) LocalAddr() net.Addr {
	return pipeAddress(f.path)
}

func (f *win32Pipe) RemoteAddr() net.Addr {
	return pipeAddress(f.path)
}

func (f *win32Pipe) SetDeadline(t time.Time) error {
	f.SetReadDeadline(t)
	f.SetWriteDeadline(t)
	return nil
}

func (s pipeAddress) Network() string {
	return "pipe"
}

func (s pipeAddress) String() string {
	return string(s)
}

func makeWin32Pipe(h syscall.Handle, path string) (*win32Pipe, error) {
	f, err := makeWin32File(h)
	if err != nil {
		return nil, err
	}
	return &win32Pipe{f, path}, nil
}

func DialPipe(s string, timeout *time.Duration) (net.Conn, error) {
	var absTimeout time.Time
	if timeout != nil {
		absTimeout = time.Now().Add(*timeout)
	}
	var err error
	var h syscall.Handle
	for {
		h, err = createFile(s, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
		if err != cERROR_PIPE_BUSY {
			break
		}
		now := time.Now()
		var ms uint32
		if absTimeout.IsZero() {
			ms = syscall.INFINITE
		} else if now.After(absTimeout) {
			ms = 1
		} else {
			ms = uint32(absTimeout.Sub(now).Nanoseconds() / 1000 / 1000)
		}
		err = waitNamedPipe(s, ms)
		if err != nil {
			if err == cERROR_SEM_TIMEOUT {
				return nil, syscall.ETIMEDOUT
			}
			break
		}
	}
	if err != nil {
		return nil, &os.PathError{"open", s, err}
	}
	p, err := makeWin32Pipe(h, s)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	return p, nil
}

type acceptResponse struct {
	p   *win32Pipe
	err error
}

type win32PipeListener struct {
	firstHandle        syscall.Handle
	path               string
	securityDescriptor string
	acceptCh           chan (chan acceptResponse)
	closeCh            chan int
	doneCh             chan int
}

func makeServerPipeHandle(path, securityDescriptor string, first bool) (syscall.Handle, error) {
	var flags uint32 = pipeFlagAccessDuplex | syscall.FILE_FLAG_OVERLAPPED
	if first {
		flags |= pipeFlagFirstInstance
	}
	var sd uintptr
	if securityDescriptor != "" {
		err := convertStringSecurityDescriptorToSecurityDescriptor(securityDescriptor, 1, &sd, nil)
		if err != nil {
			return syscall.Handle(0), err
		}
	}
	var sa syscall.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.SecurityDescriptor = sd
	h, err := createNamedPipe(path, flags, pipeModeRejectRemoteClients, pipeUnlimitedInstances, 4096, 4096, 0, &sa)
	if sd != 0 {
		localFree(sd)
	}
	if err != nil {
		return syscall.Handle(0), &os.PathError{"open", path, err}
	}
	return h, nil
}

func (l *win32PipeListener) makeServerPipe() (*win32Pipe, error) {
	h, err := makeServerPipeHandle(l.path, l.securityDescriptor, false)
	if err != nil {
		return nil, err
	}
	p, err := makeWin32Pipe(h, l.path)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	return p, nil
}

func (l *win32PipeListener) listenerRoutine() {
	closed := false
	for !closed {
		select {
		case <-l.closeCh:
			closed = true
		case responseCh := <-l.acceptCh:
			p, err := l.makeServerPipe()
			if err == nil {
				// Wait for the client to connect.
				ch := make(chan error)
				go func() {
					ch <- connectPipe(p)
				}()
				select {
				case err = <-ch:
					if err != nil {
						p.Close()
						p = nil
					}
				case <-l.closeCh:
					// Abort the connect request by closing the handle.
					p.Close()
					p = nil
					err = <-ch
					if err == nil {
						err = FileClosed
					}
					closed = true
				}
			}
			responseCh <- acceptResponse{p, err}
		}
	}
	syscall.CloseHandle(l.firstHandle)
	l.firstHandle = syscall.Handle(0)
	// Notify Close() and Accept() callers that the handle has been closed.
	close(l.doneCh)
}

func ListenPipe(path, securityDescriptor string) (net.Listener, error) {
	h, err := makeServerPipeHandle(path, securityDescriptor, true)
	if err != nil {
		return nil, err
	}
	// Immediately open and then close a client handle so that the named pipe is
	// created but not currently accepting connections.
	h2, err := createFile(path, 0, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	syscall.CloseHandle(h2)
	l := &win32PipeListener{
		firstHandle:        h,
		path:               path,
		securityDescriptor: securityDescriptor,
		acceptCh:           make(chan (chan acceptResponse)),
		closeCh:            make(chan int),
		doneCh:             make(chan int),
	}
	go l.listenerRoutine()
	return l, nil
}

func connectPipe(p *win32Pipe) error {
	c, err := p.prepareIo()
	if err != nil {
		return err
	}
	err = connectNamedPipe(p.handle, &c.o)
	_, err = p.asyncIo(c, time.Time{}, 0, err)
	if err != nil && err != cERROR_PIPE_CONNECTED {
		return err
	}
	return nil
}

func (l *win32PipeListener) Accept() (net.Conn, error) {
	ch := make(chan acceptResponse)
	select {
	case l.acceptCh <- ch:
		response := <-ch
		return response.p, response.err
	case <-l.doneCh:
		return nil, FileClosed
	}
}

func (l *win32PipeListener) Close() error {
	select {
	case l.closeCh <- 1:
		<-l.doneCh
	case <-l.doneCh:
	}
	return nil
}

func (l *win32PipeListener) Addr() net.Addr {
	return pipeAddress(l.path)
}
