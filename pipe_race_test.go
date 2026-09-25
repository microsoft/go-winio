//go:build windows

package winio

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func TestAcceptBrokenPipeRace(t *testing.T) {
	path := `\\.\pipe\test-broken-pipe-race`
	l, err := ListenPipe(path, nil)
	if err != nil {
		t.Fatalf("ListenPipe failed: %v", err)
	}
	
	done := make(chan struct{})
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				if err == ErrPipeListenerClosed || err == net.ErrClosed {
					break
				}
				panic(fmt.Sprintf("Accept failed: %v", err))
			}
			conn.Close()
		}
		close(done)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := DialPipe(path, nil)
			if err == nil {
				conn.Close()
			}
		}()
	}

	wg.Wait()
	l.Close()
	
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out")
	}
}
