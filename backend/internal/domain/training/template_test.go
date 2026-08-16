package training

import "testing"

func TestBuiltinTemplatesAreValid(t *testing.T) {
	tpls := BuiltinTemplates()
	if len(tpls) == 0 {
		t.Fatal("expected built-in templates")
	}

	seen := map[string]bool{}
	for _, tpl := range tpls {
		if tpl.ID == "" || tpl.Name == "" {
			t.Fatalf("template missing identity: %+v", tpl)
		}
		if seen[tpl.ID] {
			t.Fatalf("duplicate template id %q", tpl.ID)
		}
		seen[tpl.ID] = true

		spec := tpl.NewSpec()
		if err := spec.Validate(); err != nil {
			t.Fatalf("template %q produces an invalid spec: %v", tpl.ID, err)
		}
		if spec.TemplateID != tpl.ID {
			t.Fatalf("template %q must stamp its id onto the spec, got %q", tpl.ID, spec.TemplateID)
		}
	}
}

func TestFindTemplate(t *testing.T) {
	tpls := BuiltinTemplates()
	want := tpls[0].ID

	got, ok := FindTemplate(want)
	if !ok {
		t.Fatalf("template %q must be findable", want)
	}
	if got.ID != want {
		t.Fatalf("FindTemplate returned %q", got.ID)
	}

	if _, ok := FindTemplate("no-such-template"); ok {
		t.Fatal("unknown template id must not be found")
	}
}

func TestTemplateApplyDoesNotOverrideUserValues(t *testing.T) {
	tpl, ok := FindTemplate(TemplatePyTorchDDP)
	if !ok {
		t.Fatalf("missing template %q", TemplatePyTorchDDP)
	}

	user := Spec{Image: "my-registry/custom:1.0", GPUs: 4}
	merged := tpl.Apply(user)

	if merged.Image != "my-registry/custom:1.0" {
		t.Fatalf("template overwrote the user image: %q", merged.Image)
	}
	if merged.GPUs != 4 {
		t.Fatalf("template overwrote user GPUs: %d", merged.GPUs)
	}
	if !merged.Distributed {
		t.Fatal("template must fill in the fields the user left empty")
	}
	if merged.TemplateID != TemplatePyTorchDDP {
		t.Fatalf("merged spec must record its template, got %q", merged.TemplateID)
	}
}