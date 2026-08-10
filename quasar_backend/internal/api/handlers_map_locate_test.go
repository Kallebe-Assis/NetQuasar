package api

import "testing"

func TestParseLatLngPair(t *testing.T) {
	lat, lng, ok := parseLatLngPair("-21.408157660994526, -42.19339017156855")
	if !ok {
		t.Fatal("expected ok")
	}
	if lat > -21.4 || lat < -21.41 {
		t.Fatalf("lat=%v", lat)
	}
	if lng > -42.19 || lng < -42.2 {
		t.Fatalf("lng=%v", lng)
	}
}

func TestParseCoordsFromMapsURL(t *testing.T) {
	cases := []string{
		"https://www.google.com/maps/@-21.40815,-42.19339,17z",
		"https://www.google.com/maps?q=-21.40815,-42.19339",
		"https://www.google.com/maps/place/Foo/@-21.40815,-42.19339,17z/data=!3d-21.40815!4d-42.19339",
	}
	for _, c := range cases {
		lat, lng, ok := parseCoordsFromMapsURL(c)
		if !ok {
			t.Fatalf("failed: %s", c)
		}
		if !validLatLng(lat, lng) {
			t.Fatalf("invalid: %v %v from %s", lat, lng, c)
		}
	}
}
