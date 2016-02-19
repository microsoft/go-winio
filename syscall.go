package winio

//go:generate go run $GOROOT/src/syscall/mksyscall_windows.go -output zsyscall.go file.go pipe.go sd.go backup.go fileinfo.go privilege.go
