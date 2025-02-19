package main

import "sdk/client"

func main() {
	err := client.Register("sander@gmail.com", "81377662", "BB2")
	if err != nil {
		panic(err)
	}
}
