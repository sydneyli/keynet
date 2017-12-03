package common

import (
	"strconv"
)

func GetHostname(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}
