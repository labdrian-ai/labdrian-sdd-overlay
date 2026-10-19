package jsonstrict

import (
	"strings"
	"testing"
)

func TestCheckUTF8RejectsInvalidBytes(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "valid ASCII", data: []byte(`{"a":1}`), wantErr: false},
		{name: "invalid UTF-8 byte sequence", data: []byte{0x7b, 0xff, 0xfe, 0x7d}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckUTF8(tt.data)
			if tt.wantErr && err == nil {
				t.Fatalf("CheckUTF8() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("CheckUTF8() = %v, want nil", err)
			}
			if tt.wantErr && !strings.Contains(err.Error(), "not valid UTF-8") {
				t.Fatalf("CheckUTF8() error = %q, want message containing %q", err.Error(), "not valid UTF-8")
			}
		})
	}
}

func TestCheckNoDuplicateKeysRejectsDuplicatesAndTrailingData(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{name: "no duplicates", data: `{"a":1,"b":2}`, wantErr: false},
		{name: "duplicate key at top level", data: `{"a":1,"a":2}`, wantErr: true},
		{name: "duplicate key nested inside object", data: `{"a":{"b":1,"b":2}}`, wantErr: true},
		{name: "duplicate key inside object within array", data: `{"a":[{"b":1,"b":2}]}`, wantErr: true},
		{name: "trailing data after top-level value", data: `{"a":1}{"b":2}`, wantErr: true},
		{name: "trailing scalar after top-level value", data: `{"a":1} 2`, wantErr: true},
		{name: "empty document", data: ``, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckNoDuplicateKeys([]byte(tt.data))
			if tt.wantErr && err == nil {
				t.Fatalf("CheckNoDuplicateKeys(%q) = nil, want error", tt.data)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("CheckNoDuplicateKeys(%q) = %v, want nil", tt.data, err)
			}
		})
	}
}

func TestCheckKnownFieldsRejectsUnknownAndNonObjectRoot(t *testing.T) {
	allowed := []string{"a", "b"}
	tests := []struct {
		name       string
		data       string
		recordName string
		wantErr    bool
		wantMsg    string
	}{
		{name: "allowed fields accepted", data: `{"a":1,"b":2}`, recordName: "Widget", wantErr: false},
		{name: "unknown top-level field rejected", data: `{"a":1,"c":3}`, recordName: "Widget", wantErr: true, wantMsg: "unknown Widget field"},
		{name: "non-object root array", data: `[1,2,3]`, recordName: "Widget", wantErr: true},
		{name: "non-object root string", data: `"hello"`, recordName: "Widget", wantErr: true},
		{name: "non-object root number", data: `42`, recordName: "Widget", wantErr: true},
		{name: "non-object root null", data: `null`, recordName: "Widget", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckKnownFields([]byte(tt.data), tt.recordName, allowed)
			if tt.wantErr && err == nil {
				t.Fatalf("CheckKnownFields(%q) = nil, want error", tt.data)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("CheckKnownFields(%q) = %v, want nil", tt.data, err)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("CheckKnownFields(%q) error = %q, want message containing %q", tt.data, err.Error(), tt.wantMsg)
			}
		})
	}
}

// A document with several unknown fields is refused with the same error every time: the one
// that names the smallest of them by bytes. The unknown fields come out of a map, whose order
// Go makes random on purpose, so the test asks 200 times and counts the distinct errors.
func TestCheckKnownFieldsNamesTheSmallestUnknownFieldWhateverTheOrderOfTheDocument(t *testing.T) {
	allowed := []string{"a", "b"}
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "the smallest is not the first of the document",
			data: `{"zeta":1,"a":1,"mid":2,"alpha":3,"omega":4,"beta":5,"b":2,"kappa":6}`,
			want: `unknown Widget field "alpha"`,
		},
		{
			name: "the smallest is the last of the document",
			data: `{"x3":1,"x2":2,"x1":3,"x0":4,"a":1,"b":2}`,
			want: `unknown Widget field "x0"`,
		},
		{
			name: "the order is by bytes, so a capital comes before a lowercase letter",
			data: `{"alpha":1,"Zed":2,"beta":3,"gamma":4,"delta":5}`,
			want: `unknown Widget field "Zed"`,
		},
		{
			name: "a name that is a prefix of another comes before it",
			data: `{"field_b":1,"field":2,"field_a":3,"fiel":4}`,
			want: `unknown Widget field "fiel"`,
		},
		{
			name: "a known field is never the one named, even when it sorts first",
			data: `{"a":1,"b":2,"c":3,"d":4,"e":5}`,
			want: `unknown Widget field "c"`,
		},
		{
			name: "one unknown field is named as before",
			data: `{"a":1,"c":3}`,
			want: `unknown Widget field "c"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := map[string]int{}
			for i := 0; i < 200; i++ {
				err := CheckKnownFields([]byte(tt.data), "Widget", allowed)
				if err == nil {
					t.Fatalf("CheckKnownFields(%s) = nil, want an error", tt.data)
				}
				seen[err.Error()]++
			}
			if len(seen) != 1 || seen[tt.want] != 200 {
				t.Errorf("CheckKnownFields(%s) in 200 runs gave %v, want %q every time", tt.data, seen, tt.want)
			}
		})
	}
}

func TestDecodeStrictRejectsTrailingDataAndDecodesValidDocument(t *testing.T) {
	type widget struct {
		A int `json:"a"`
	}

	t.Run("valid document decodes successfully", func(t *testing.T) {
		var w widget
		if err := DecodeStrict([]byte(`{"a":1}`), "Widget", &w); err != nil {
			t.Fatalf("DecodeStrict() = %v, want nil", err)
		}
		if w.A != 1 {
			t.Fatalf("DecodeStrict() decoded A = %d, want 1", w.A)
		}
	})

	t.Run("trailing data after top-level value rejected", func(t *testing.T) {
		var w widget
		err := DecodeStrict([]byte(`{"a":1}{"a":2}`), "Widget", &w)
		if err == nil {
			t.Fatalf("DecodeStrict() = nil, want error")
		}
		if !strings.Contains(err.Error(), "trailing data after the Widget value") {
			t.Fatalf("DecodeStrict() error = %q, want message containing %q", err.Error(), "trailing data after the Widget value")
		}
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		var w widget
		err := DecodeStrict([]byte(`{"a":1,"c":2}`), "Widget", &w)
		if err == nil {
			t.Fatalf("DecodeStrict() = nil, want error")
		}
	})
}

// WireVersion names a version only for a top-level, exact-case version member written as a
// canonical integer; every other spelling, and a document that is not an object, is 0.
func TestWireVersionReadsOnlyACanonicalIntegerVersion(t *testing.T) {
	tests := []struct {
		data string
		want int
	}{
		{`{"version":3}`, 3},
		{`{"version": 4 ,"a":1}`, 4},
		{`{"version":3.0}`, 0},
		{`{"version":3e0}`, 0},
		{`{"version":03}`, 0},
		{`{"version":-0}`, 0},
		{`{"version":+3}`, 0},
		{`{"version":3_0}`, 0},
		{`{"version":null}`, 0},
		{`{"version":"3"}`, 0},
		{`{"Version":3}`, 0},
		{`{"a":1}`, 0},
		{`[3]`, 0},
		{`not json`, 0},
	}
	for _, tt := range tests {
		if got := WireVersion([]byte(tt.data)); got != tt.want {
			t.Errorf("WireVersion(%s) = %d, want %d", tt.data, got, tt.want)
		}
	}
}
