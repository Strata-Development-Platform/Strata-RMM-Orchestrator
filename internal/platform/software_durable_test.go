package platform

import "testing"

func TestNormalizeSoftwareDeviceIDs(t *testing.T) {
	got := normalizeSoftwareDeviceIDs([]string{" device-a ", "", "device-a", "device-b", "  ", "device-b"})
	want := []string{"device-a", "device-b"}
	if len(got) != len(want) {
		t.Fatalf("normalized IDs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized IDs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
