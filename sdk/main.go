package main

import "sdk/client"

func main() {
	err := client.Register("alex@gmail.com", "81377662", "NEWDB")
	if err != nil {
		panic(err)
	}
}
