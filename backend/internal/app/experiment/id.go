package experiment

import (
	"fmt"
	"time"
)

func generateID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), fastRand())
}

func fastRand() uint32 {
	
	return uint32(time.Now().UnixNano() & 0xFFFFF)
}