package utils

import (
	"fmt"
	"net/url"
	"reflect"
)

func HttpTimeout(err error) bool {
	httpErr := err.(*url.Error)
	if httpErr != nil {
		return httpErr.Timeout()
	}
	return false
}

func Contains(val interface{}, slice interface{}) bool {
	if slice == nil {
		return false
	}
	for i := 0; i < reflect.ValueOf(slice).Len(); i++ {
		if fmt.Sprintf("%v", reflect.ValueOf(val).Interface()) == fmt.Sprintf("%v", reflect.ValueOf(slice).Index(i).Interface()) {
			return true
		}
	}
	return false
}
