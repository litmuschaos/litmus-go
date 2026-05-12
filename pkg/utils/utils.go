package utils

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"syscall"
)

func HttpTimeout(err error) bool {
	httpErr := err.(*url.Error)
	if httpErr != nil {
		return httpErr.Timeout()
	}
	return false
}

func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}

	var syscallErr syscall.Errno
	if errors.As(err, &syscallErr) {
		switch syscallErr {
		case syscall.ECONNREFUSED, syscall.ETIMEDOUT, syscall.ENETUNREACH,
			syscall.EHOSTUNREACH, syscall.ECONNRESET, syscall.EPIPE:
			return true
		}
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
