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
