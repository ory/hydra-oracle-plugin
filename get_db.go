package main

import "strings"

func GetDatabase(u string) (string, string) {
	split := strings.Split(u, "/")
	database := split[len(split)-1]
	host := strings.Join(split[:len(split)-1], "/")
	return host, database
}
