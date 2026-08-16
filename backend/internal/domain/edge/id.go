package edge

import "github.com/google/uuid"

func generateID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}