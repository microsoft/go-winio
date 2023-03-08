package osversion

import (
	"fmt"
	"testing"
)

func TestCompare(t *testing.T) {
	tt := []struct {
		a, b Version
		res  int
	}{
		{
			Version{10, 0, RS5},
			Version{10, 0, LTSC2022},
			-1,
		},
		{
			Version{6, 1, 9801},
			Version{10, 0, LTSC2022},
			-1,
		},
		{
			Version{10, 0, RS5},
			Version{10, 0, RS5},
			0,
		},
		{
			Version{10, 0, LTSC2022},
			Version{10, 0, RS5},
			1,
		},
		{
			Version{10, 0, LTSC2022},
			Version{6, 1, 9801},
			1,
		},
	}

	for _, tc := range tt {
		if res := tc.a.Compare(tc.b); res != tc.res {
			t.Errorf("(%s).Compare(%s): expected: %d, got: %d", tc.a, tc.b, res, tc.res)
		}
	}
}

func TestOSVersionString(t *testing.T) {
	v := FromPackedVersion(809042555)
	expected := "123.2.12345"
	actual := fmt.Sprintf("%s", v) //nolint: gosimple // testing that fmt works
	if actual != expected {
		t.Errorf("expected: %q, got: %q", expected, actual)
	}
}
