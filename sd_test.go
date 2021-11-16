//go:build windows
// +build windows

package winio

import (
	"testing"
)

func TestLookupInvalidSid(t *testing.T) {
	_, err := LookupSidByName(".\\weoifjdsklfj")
	aerr, ok := err.(*AccountLookupError)
	if !ok || aerr.Err != cERROR_NONE_MAPPED {
		t.Fatalf("expected AccountLookupError with ERROR_NONE_MAPPED, got %s", err)
	}
}

func TestLookupValidSid(t *testing.T) {
	sid, err := LookupSidByName("Everyone")
	if err != nil || sid != "S-1-1-0" {
		t.Fatalf("expected S-1-1-0, got %s, %s", sid, err)
	}
}

func TestLookupEmptyNameFails(t *testing.T) {
	_, err := LookupSidByName("")
	aerr, ok := err.(*AccountLookupError)
	if !ok || aerr.Err != cERROR_NONE_MAPPED {
		t.Fatalf("expected AccountLookupError with ERROR_NONE_MAPPED, got %s", err)
	}
}

func TestGetFileSDDL(t *testing.T) {
	win32 := `C:\Windows\System32`
	kern := `C:\Windows\System32\kernel32.dll`

	err := EnableProcessPrivileges([]string{SeBackupPrivilege})
	if err != nil {
		t.Fatalf("failed to gain privileges: %s", err)
	}

	_, err = GetFileSecurityDescriptor(win32)
	if err != nil {
		t.Fatalf("failed to get SD for %s: %s", win32, err)
	}

	_, err = GetFileSecurityDescriptor(kern)
	if err != nil {
		t.Fatalf("failed to get SD for %s: %s", kern, err)
	}
}
