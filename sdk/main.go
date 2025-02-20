package main

import "sdk/client"

func main() {
	err := client.Register("sander@gmail.com", "81377662", "akaksk")
	if err != nil {
		panic(err)
	}
}
