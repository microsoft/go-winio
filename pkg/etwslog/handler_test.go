//go:build windows

package etwslog

import (
	"log/slog"
	"testing"
)

func fireEvent(logger *slog.Logger, name string, value any) {
	logger.Info(name, "Field", value)
}

// The purpose of this test is to log lots of different field types, to test the
// logic that converts them to ETW. Because we don't have a way to
// programatically validate the ETW events, this test has two main purposes: (1)
// validate nothing causes a panic while logging (2) allow manual validation that
// the data is logged correctly (through a tool like WPA).
func TestFieldLogging(t *testing.T) {
	// Sample WPRP to collect this provider is included in HookTest.wprp.
	//
	// Start collection:
	// wpr -start HookTest.wprp -filemode
	//
	// Stop collection:
	// wpr -stop HookTest.etl
	h, err := NewHandler("HookTest")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	logger := slog.New(h)

	fireEvent(logger, "Bool", true)
	fireEvent(logger, "BoolSlice", []bool{true, false, true})
	fireEvent(logger, "EmptyBoolSlice", []bool{})
	fireEvent(logger, "String", "teststring")
	fireEvent(logger, "StringSlice", []string{"sstr1", "sstr2", "sstr3"})
	fireEvent(logger, "EmptyStringSlice", []string{})
	fireEvent(logger, "Int", int(1))
	fireEvent(logger, "IntSlice", []int{2, 3, 4})
	fireEvent(logger, "EmptyIntSlice", []int{})
	fireEvent(logger, "Int8", int8(5))
	fireEvent(logger, "Int8Slice", []int8{6, 7, 8})
	fireEvent(logger, "EmptyInt8Slice", []int8{})
	fireEvent(logger, "Int16", int16(9))
	fireEvent(logger, "Int16Slice", []int16{10, 11, 12})
	fireEvent(logger, "EmptyInt16Slice", []int16{})
	fireEvent(logger, "Int32", int32(13))
	fireEvent(logger, "Int32Slice", []int32{14, 15, 16})
	fireEvent(logger, "EmptyInt32Slice", []int32{})
	fireEvent(logger, "Int64", int64(17))
	fireEvent(logger, "Int64Slice", []int64{18, 19, 20})
	fireEvent(logger, "EmptyInt64Slice", []int64{})
	fireEvent(logger, "Uint", uint(21))
	fireEvent(logger, "UintSlice", []uint{22, 23, 24})
	fireEvent(logger, "EmptyUintSlice", []uint{})
	fireEvent(logger, "Uint8", uint8(25))
	fireEvent(logger, "Uint8Slice", []uint8{26, 27, 28})
	fireEvent(logger, "EmptyUint8Slice", []uint8{})
	fireEvent(logger, "Uint16", uint16(29))
	fireEvent(logger, "Uint16Slice", []uint16{30, 31, 32})
	fireEvent(logger, "EmptyUint16Slice", []uint16{})
	fireEvent(logger, "Uint32", uint32(33))
	fireEvent(logger, "Uint32Slice", []uint32{34, 35, 36})
	fireEvent(logger, "EmptyUint32Slice", []uint32{})
	fireEvent(logger, "Uint64", uint64(37))
	fireEvent(logger, "Uint64Slice", []uint64{38, 39, 40})
	fireEvent(logger, "EmptyUint64Slice", []uint64{})
	fireEvent(logger, "Uintptr", uintptr(41))
	fireEvent(logger, "UintptrSlice", []uintptr{42, 43, 44})
	fireEvent(logger, "EmptyUintptrSlice", []uintptr{})
	fireEvent(logger, "Float32", float32(45.46))
	fireEvent(logger, "Float32Slice", []float32{47.48, 49.50, 51.52})
	fireEvent(logger, "EmptyFloat32Slice", []float32{})
	fireEvent(logger, "Float64", float64(53.54))
	fireEvent(logger, "Float64Slice", []float64{55.56, 57.58, 59.60})
	fireEvent(logger, "EmptyFloat64Slice", []float64{})

	type struct1 struct {
		A    float32
		priv int
		B    []uint
	}
	type struct2 struct {
		A int
		B int
	}
	type struct3 struct {
		struct2
		A    int
		B    string
		priv string
		C    struct1
		D    uint16
	}
	// Unexported fields, and fields in embedded structs, should not log.
	fireEvent(logger, "Struct", struct3{struct2{-1, -2}, 1, "2s", "-3s", struct1{3.4, -4, []uint{5, 6, 7}}, 8})
}
