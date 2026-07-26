package models

import (
	"reflect"
	"strings"
	"testing"
)

func TestStringArrayScanAcceptsSQLiteTextShapes(t *testing.T) {
	tests := map[string]interface{}{
		"bytes": []byte(`["a","b"]`),
		"text":  `["a","b"]`,
		"empty": "",
		"nil":   nil,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			var got StringArray
			if err := got.Scan(input); err != nil {
				t.Fatalf("scan StringArray: %v", err)
			}
			if input == "" || input == nil {
				if len(got) != 0 {
					t.Fatalf("expected empty StringArray, got %v", got)
				}
				return
			}
			if len(got) != 2 || got[0] != "a" || got[1] != "b" {
				t.Fatalf("unexpected StringArray: %v", got)
			}
		})
	}
}

func TestClientTrafficDefaults(t *testing.T) {
	clientType := reflect.TypeOf(Client{})
	tests := map[string]string{
		"TrafficLimitType":  "default:'sum'",
		"TrafficMultiplier": "default:1",
	}

	for fieldName, expected := range tests {
		field, ok := clientType.FieldByName(fieldName)
		if !ok {
			t.Fatalf("Client.%s field is missing", fieldName)
		}
		if tag := field.Tag.Get("gorm"); !strings.Contains(tag, expected) {
			t.Fatalf("Client.%s gorm tag %q does not contain %q", fieldName, tag, expected)
		}
	}
}
