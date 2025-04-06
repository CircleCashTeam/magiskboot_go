package magiskboot

import "os"

func checkenv(key string) bool {
	value, ret := os.LookupEnv(key)
	if ret {
		if value == "true" {
			return true
		}
	}
	return false
}
