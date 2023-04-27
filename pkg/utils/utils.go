package utils

import "net/url"

func HttpTimeout(err error) bool {
	httpErr := err.(*url.Error)
	if httpErr != nil {
		return httpErr.Timeout()
	}
	return false
}
