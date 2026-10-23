package transcode

import "testing"

func TestParseFramePTS(t *testing.T) {
	out := "#tb 0: 1/1000\n#media_type 0: video\n0,      19925,      20005,       40,     4652, 0xee3b33a9\n"
	if v, ok := parseFramePTS(out); !ok || v != 19.925 {
		t.Fatalf("got %v %v", v, ok)
	}
	if v, ok := parseFramePTS("#tb 0: 1/1000\n0,        -75,          5,       40,     4826, 0x462501ad\n"); !ok || v != 0 {
		t.Fatalf("negative pts: got %v %v", v, ok)
	}
	if _, ok := parseFramePTS(""); ok {
		t.Fatal("empty output parsed")
	}
}
