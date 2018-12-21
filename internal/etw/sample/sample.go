// Shows a sample usage of the ETW logging package.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Microsoft/go-winio/internal/etw"
	"github.com/sirupsen/logrus"

	"golang.org/x/sys/windows"
)

func callback(sourceID *windows.GUID, state etw.ProviderState, level etw.Level, matchAnyKeyword uint64, matchAllKeyword uint64, filterData uintptr) {
	fmt.Printf("Callback: isEnabled=%d, level=%d, matchAnyKeyword=%d\n", state, level, matchAnyKeyword)
}

func guidToString(guid *windows.GUID) string {
	data1 := make([]byte, 4)
	binary.BigEndian.PutUint32(data1, guid.Data1)
	data2 := make([]byte, 2)
	binary.BigEndian.PutUint16(data2, guid.Data2)
	data3 := make([]byte, 2)
	binary.BigEndian.PutUint16(data3, guid.Data3)
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s",
		hex.EncodeToString(data1),
		hex.EncodeToString(data2),
		hex.EncodeToString(data3),
		hex.EncodeToString(guid.Data4[:2]),
		hex.EncodeToString(guid.Data4[2:]))
}

func main() {
	provider, err := etw.NewProvider("TestProvider", callback)

	if err != nil {
		logrus.Error(err)
		return
	}
	defer func() {
		if err := provider.Close(); err != nil {
			logrus.Error(err)
		}
	}()

	fmt.Println("Provider ID:", guidToString(provider.ID))

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Press enter to log an event")
	reader.ReadString('\n')

	event := etw.NewEvent("TestEvent", etw.NewEventDescriptor())
	event.Metadata.AddField("TestField", etw.InTypeANSIString)
	event.Data.AddString("Foo")
	event.Metadata.AddField("TestField2", etw.InTypeANSIString)
	event.Data.AddString("Bar")
	event.Metadata.AddField("TestArray", etw.InTypeANSIString, etw.WithArray())
	event.Data.AddSimple(uint16(5))
	event.Data.AddString("Item1")
	event.Data.AddString("Item2")
	event.Data.AddString("Item3")
	event.Data.AddString("Item4")
	event.Data.AddString("Item5")

	if err := provider.WriteEvent(event); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Event written")

	fmt.Println("Press enter to exit")
	reader.ReadString('\n')
}
