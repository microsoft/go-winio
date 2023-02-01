package fs

// https://learn.microsoft.com/en-us/windows/win32/api/winnt/ne-winnt-security_impersonation_level
type SecurityImpersonationLevel int32 // C++ default enums underlying type is `int`, which is Go `int32`

// Impersonation levels
const (
	SecurityAnonymous      = 0
	SecurityIdentification = 1
	SecurityImpersonation  = 2
	SecurityDelegation     = 3
)
