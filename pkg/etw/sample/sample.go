// Shows a sample usage of the ETW logging package.
package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/Microsoft/go-winio/pkg/etw"
	"github.com/sirupsen/logrus"

	"golang.org/x/sys/windows"
)

func callback(sourceID *windows.GUID, state etw.ProviderState, level etw.Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr) {
	fmt.Printf("Callback: isEnabled=%d, level=%d, matchAnyKeyword=%d\n", state, level, matchAnyKeyword)
}

func main() {
	providerID := windows.GUID{0xdd2062c6, 0x5d1b, 0x4a0f, [8]uint8{0xbd, 0xb9, 0x22, 0x28, 0xbc, 0xb1, 0x07, 0x7c}}

	provider, err := etw.NewProvider("TestProvider", &providerID, callback)
	if err != nil {
		logrus.Error(err)
		return
	}
	defer func() {
		if err := provider.Close(); err != nil {
			logrus.Error(err)
		}
	}()

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Press enter to log an event")
	reader.ReadString('\n')

	event := etw.NewEvent("TestEvent", etw.NewEventDescriptor())
	event.Metadata.AddField("TestField", etw.InTypeAnsiString)
	event.Data.AddString("Foo")
	event.Metadata.AddField("TestField2", etw.InTypeAnsiString)
	event.Data.AddString("Bar")

	if err := provider.WriteEvent(event); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Event written")

	fmt.Println("Press enter to exit")
	reader.ReadString('\n')
}
