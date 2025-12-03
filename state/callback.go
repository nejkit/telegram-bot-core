package state

import (
	"fmt"
	"strings"
)

type CallbackPrefix interface {
	~string
}

func WrapCallbackData[T CallbackPrefix](prefix T, data string) string {
	return fmt.Sprintf("%s_%s", prefix, data)
}

func UnwrapCallbackData[T CallbackPrefix](data string) (T, string) {
	dataParts := strings.SplitN(data, "_", 2)

	if len(dataParts) != 2 {
		return T(""), ""
	}

	return T(dataParts[0]), dataParts[1]
}
