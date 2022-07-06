//go:build windows
// +build windows

package winio

import "testing"

func TestLookupInvalidSid(t *testing.T) {
	_, err := LookupSidByName(".\\weoifjdsklfj")
	aerr, ok := err.(*AccountLookupError)
	if !ok || aerr.Err != cERROR_NONE_MAPPED {
		t.Fatalf("expected AccountLookupError with ERROR_NONE_MAPPED, got %s", err)
	}
}

func TestLookupInvalidName(t *testing.T) {
	_, err := LookupNameBySid("notasid")
	aerr, ok := err.(*AccountLookupError)
	if !ok || aerr.Err != cERROR_INVALID_SID {
		t.Fatalf("expected AccountLookupError with ERROR_INVALID_SID got %s", err)
	}
}

func TestLookupValidSid(t *testing.T) {
	everyone := "S-1-1-0"
	name, err := LookupNameBySid(everyone)
	if err != nil {
		t.Fatalf("expected a valid account name, got %v", err)
	}

	sid, err := LookupSidByName(name)
	if err != nil || sid != everyone {
		t.Fatalf("expected %s, got %s, %s", everyone, sid, err)
	}
}

func TestLookupEmptyNameFails(t *testing.T) {
	_, err := LookupSidByName("")
	aerr, ok := err.(*AccountLookupError)
	if !ok || aerr.Err != cERROR_NONE_MAPPED {
		t.Fatalf("expected AccountLookupError with ERROR_NONE_MAPPED, got %s", err)
	}
}
