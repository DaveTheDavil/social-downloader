package downloader

import (
	"reflect"
	"testing"
)

func TestAvailableQualitiesPuste(t *testing.T) {
	if got := AvailableQualities(nil); got != nil {
		t.Errorf("AvailableQualities(nil) = %v, chciano nil/puste", got)
	}
	if got := AvailableQualities([]Format{}); len(got) != 0 {
		t.Errorf("AvailableQualities(puste) = %v, chciano []", got)
	}
}

func TestAvailableQualitiesIgnorujeDzwiekIWysokoscZero(t *testing.T) {
	formats := []Format{
		{FormatID: "140", Height: 0, VCodec: "none", ACodec: "mp4a.40.2"},   // audio tylko
		{FormatID: "160", Height: 0, VCodec: "avc1.42c00d", ACodec: "none"}, // wideo bez wysokości
		{FormatID: "137", Height: 1080, VCodec: "avc1.640028", ACodec: "none"},
		{FormatID: "22", Height: 720, VCodec: "avc1.64001f", ACodec: "mp4a.40.2"},
	}
	got := AvailableQualities(formats)
	want := []int{720, 1080}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AvailableQualities = %v, chciano %v", got, want)
	}
}

func TestAvailableQualitiesDeduplikujeSortuje(t *testing.T) {
	formats := []Format{
		{FormatID: "137", Height: 1080, VCodec: "avc1", ACodec: "none"},
		{FormatID: "136", Height: 720, VCodec: "avc1", ACodec: "none"},
		{FormatID: "135", Height: 480, VCodec: "avc1", ACodec: "none"},
		{FormatID: "134", Height: 360, VCodec: "avc1", ACodec: "none"},
		{FormatID: "137dup", Height: 1080, VCodec: "avc1", ACodec: "none"}, // duplikat 1080
		{FormatID: "22", Height: 720, VCodec: "avc1", ACodec: "mp4a"},      // duplikat 720
	}
	got := AvailableQualities(formats)
	want := []int{360, 480, 720, 1080}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AvailableQualities = %v, chciano %v", got, want)
	}
}

func TestAvailableQualitiesJedenFormat(t *testing.T) {
	formats := []Format{
		{FormatID: "18", Height: 360, VCodec: "avc1.42001e", ACodec: "mp4a.40.2"},
	}
	got := AvailableQualities(formats)
	want := []int{360}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AvailableQualities = %v, chciano %v", got, want)
	}
}
