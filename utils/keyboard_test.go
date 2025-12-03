package utils

import (
	"encoding/json"
	"testing"
)

func TestBuildInlineDataKeyboard(t *testing.T) {
	sortedKeys := []string{"TestTitle"}
	data := map[string]string{
		"TestTitle": "TestValue",
	}

	kbrd := BuildInlineDataKeyboard(sortedKeys, data, 3)

	str, _ := json.Marshal(kbrd)

	t.Log(string(str))
}
