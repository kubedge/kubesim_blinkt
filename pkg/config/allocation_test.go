package config

import (
	"reflect"
	"testing"
)

func TestParsePixels(t *testing.T) {
	for in, want := range map[string][]int{"6": {6}, "0,1,2,3,4,5,6,7": {0, 1, 2, 3, 4, 5, 6, 7}, " 2 , 4": {2, 4}} {
		got, err := ParsePixels(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParsePixels(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "9", "-1", "a", "1,,2"} {
		if _, err := ParsePixels(bad); err == nil || err.Error() != "invalid BLINKT_PIXELS: "+bad {
			t.Errorf("ParsePixels(%q) error = %v", bad, err)
		}
	}
}

func TestAllocate(t *testing.T) {
	blue := RGB{0, 0, 255}
	green := RGB{0, 255, 0}
	conf := map[int]RGB{6: blue}
	if got := Allocate(conf, []int{6}); !reflect.DeepEqual(got, map[int]RGB{6: blue}) {
		t.Errorf("matching allocation = %v", got)
	}
	if got := Allocate(conf, []int{2}); !reflect.DeepEqual(got, map[int]RGB{2: blue}) {
		t.Errorf("other pixel = %v", got)
	}
	two := map[int]RGB{6: blue, 4: green}
	if got := Allocate(two, []int{1, 6}); !reflect.DeepEqual(got, map[int]RGB{1: green, 6: blue}) {
		t.Errorf("first configured colour = %v", got)
	}
	if got := Allocate(map[int]RGB{}, []int{1}); len(got) != 0 {
		t.Errorf("no colours = %v", got)
	}
}

func TestColours(t *testing.T) {
	c := BlinktConfigData{Pixel2: []int{255}, Pixel6: []int{0, 0, 255}}
	if got := c.Colours(); !reflect.DeepEqual(got, map[int]RGB{6: {0, 0, 255}}) {
		t.Errorf("Colours = %v", got)
	}
}
