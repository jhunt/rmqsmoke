package main

import (
	"time"
)

func nap(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
