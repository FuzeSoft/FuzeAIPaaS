
package main

import (
	"encoding/json"
	"log"
	"os"

	"fuze-ai-paas/backend/internal/data/operator"
)

func main() {
	raw := os.Getenv("FUZE_DATA_SPEC")
	if raw == "" {
		log.Fatal("FUZE_DATA_SPEC is empty; data-operator requires step spec")
	}
	var spec operator.Spec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		log.Fatalf("invalid FUZE_DATA_SPEC json: %v", err)
	}
	log.Printf("data-operator: operator=%q input=%q output=%q", spec.Operator, spec.Input, spec.Output)
	if err := operator.Run(spec); err != nil {
		log.Fatalf("operator %q failed: %v", spec.Operator, err)
	}
	os.Stdout.WriteString("data-operator: done\n")
}