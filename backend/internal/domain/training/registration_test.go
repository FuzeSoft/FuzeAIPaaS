package training

import "testing"

func TestModelRegistrationValidate(t *testing.T) {
	
	if err := (ModelRegistration{}).Validate(); err != nil {
		t.Fatalf("disabled registration rejected: %v", err)
	}

	if err := (ModelRegistration{Enabled: true, VersionTag: "v1"}).Validate(); err == nil {
		t.Fatal("enabled registration without model id must be rejected")
	}
	if err := (ModelRegistration{Enabled: true, ModelID: "m1"}).Validate(); err == nil {
		t.Fatal("enabled registration without version tag must be rejected")
	}
	if err := (ModelRegistration{Enabled: true, ModelID: "m1", VersionTag: "v1"}).Validate(); err != nil {
		t.Fatalf("valid registration rejected: %v", err)
	}
}

func TestModelRegistrationNormalizeTrims(t *testing.T) {
	r := ModelRegistration{Enabled: true, ModelID: " m1 ", VersionTag: " v1 "}
	r.Normalize()
	if r.ModelID != "m1" || r.VersionTag != "v1" {
		t.Fatalf("fields not trimmed: %+v", r)
	}
}