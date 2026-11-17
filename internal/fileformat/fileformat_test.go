package fileformat

import (
	"encoding/json"
	"strings"
	"testing"
)

type item struct {
	ID string `json:"id"`
}

func TestDecodeLegacyArray(t *testing.T) {
	list, v, err := DecodeList[item]("/x/p.json", []byte(` [{"id":"a"},{"id":"b"}]`), "items", 1)
	if err != nil || v != 1 || len(list) != 2 || list[1].ID != "b" {
		t.Fatalf("got %v %d %v", list, v, err)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	data, err := EncodeList("items", 1, []item{{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "{\n  \"version\": 1,\n  \"items\": [") || !strings.HasSuffix(string(data), "]\n}\n") {
		t.Fatalf("layout:\n%s", data)
	}
	if !json.Valid(data) {
		t.Fatalf("invalid JSON:\n%s", data)
	}
	list, v, err := DecodeList[item]("p.json", data, "items", 1)
	if err != nil || v != 1 || len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("got %v %d %v", list, v, err)
	}
	empty, _ := EncodeList[item]("items", 1, nil)
	if list, _, err := DecodeList[item]("p.json", empty, "items", 1); err != nil || len(list) != 0 {
		t.Fatalf("empty: %v %v", list, err)
	}
}

func TestDecodeTooNew(t *testing.T) {
	_, v, err := DecodeList[item]("/x/p.json", []byte(`{"version":2,"items":[{"id":"a","new":1}]}`), "items", 1)
	if !IsTooNew(err) || v != 2 {
		t.Fatalf("got %d %v", v, err)
	}
	if !strings.Contains(err.Error(), "p.json was written by a newer agenterm (format 2; this build reads up to 1)") {
		t.Fatalf("message: %v", err)
	}
}

func TestDecodeBroken(t *testing.T) {
	for _, in := range []string{`{`, `{"items":[]}`, `{"version":"1"}`, `{"version":-1}`, `{"version":1,"items":{}}`, `"x"`} {
		if _, _, err := DecodeList[item]("p.json", []byte(in), "items", 1); err == nil || IsTooNew(err) {
			t.Errorf("%s: got %v", in, err)
		}
	}
}

func TestCheck(t *testing.T) {
	if Check("f", 0, 1) != nil || Check("f", 1, 1) != nil {
		t.Fatal("0 and 1 are accepted")
	}
	if !IsTooNew(Check("f", 2, 1)) {
		t.Fatal("2 is too new")
	}
	if err := Check("f", -1, 1); err == nil || IsTooNew(err) {
		t.Fatal("negative is invalid")
	}
}
