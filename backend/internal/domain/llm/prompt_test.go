package llm

import "testing"

func TestTemplateVariables(t *testing.T) {
	tpl := PromptTemplate{Content: "Hi {{name}}, you are {{role}}. Bye {{name}}."}
	got, err := tpl.Variables()
	if err != nil {
		t.Fatalf("Variables: %v", err)
	}
	
	if len(got) != 2 || got[0] != "name" || got[1] != "role" {
		t.Fatalf("Variables() = %v, want [name role]", got)
	}
}

func TestTemplateIgnoresSingleBraces(t *testing.T) {
	tpl := PromptTemplate{Content: `Return JSON like {"score": 1} for {{item}}`}
	vars, err := tpl.Variables()
	if err != nil {
		t.Fatalf("Variables: %v", err)
	}
	if len(vars) != 1 || vars[0] != "item" {
		t.Fatalf("Variables() = %v, want [item]", vars)
	}
	out, err := tpl.Render(map[string]string{"item": "apple"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `Return JSON like {"score": 1} for apple`
	if out != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}
}

func TestTemplateUnclosedPlaceholder(t *testing.T) {
	tpl := PromptTemplate{Content: "hello {{name"}
	if _, err := tpl.Variables(); err != ErrUnclosedPlaceholder {
		t.Fatalf("want ErrUnclosedPlaceholder, got %v", err)
	}
	if _, err := tpl.Render(map[string]string{"name": "x"}); err != ErrUnclosedPlaceholder {
		t.Fatalf("Render want ErrUnclosedPlaceholder, got %v", err)
	}
}

func TestTemplateRenderRejectsMissingVariable(t *testing.T) {
	tpl := PromptTemplate{Content: "Hi {{name}} from {{city}}"}
	if _, err := tpl.Render(map[string]string{"name": "bob"}); err != ErrMissingVariable {
		t.Fatalf("want ErrMissingVariable, got %v", err)
	}
}

func TestTemplateRenderEmpty(t *testing.T) {
	if _, err := (PromptTemplate{Content: "   "}).Render(nil); err != ErrEmptyTemplate {
		t.Fatalf("want ErrEmptyTemplate, got %v", err)
	}
}

func TestTemplateRenderTrimsPlaceholderSpaces(t *testing.T) {
	tpl := PromptTemplate{Content: "Hi {{ name }}!"}
	out, err := tpl.Render(map[string]string{"name": "bob"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out != "Hi bob!" {
		t.Fatalf("Render() = %q, want %q", out, "Hi bob!")
	}
}

func TestPromptAddVersionIncrements(t *testing.T) {
	p := &Prompt{Name: "greet"}
	v1, err := p.AddVersion("v one {{x}}", "init")
	if err != nil {
		t.Fatalf("AddVersion: %v", err)
	}
	v2, _ := p.AddVersion("v two {{x}}", "tweak")
	if v1.Version != 1 || v2.Version != 2 {
		t.Fatalf("versions = %d,%d want 1,2", v1.Version, v2.Version)
	}
}

func TestPromptAddVersionRejectsBadContent(t *testing.T) {
	p := &Prompt{Name: "p"}
	if _, err := p.AddVersion("", "x"); err != ErrEmptyTemplate {
		t.Fatalf("want ErrEmptyTemplate, got %v", err)
	}
	
	if _, err := p.AddVersion("broken {{x", "x"); err != ErrUnclosedPlaceholder {
		t.Fatalf("want ErrUnclosedPlaceholder, got %v", err)
	}
	if len(p.Versions) != 0 {
		t.Fatalf("invalid version was persisted: %+v", p.Versions)
	}
}

func TestPromptActiveDefaultsToLatest(t *testing.T) {
	p := &Prompt{Name: "p"}
	if _, err := p.Active(); err != ErrNoVersion {
		t.Fatalf("want ErrNoVersion, got %v", err)
	}
	_, _ = p.AddVersion("a", "")
	_, _ = p.AddVersion("b", "")

	got, err := p.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("Active version = %d, want 2 (latest)", got.Version)
	}
}

func TestPromptActivate(t *testing.T) {
	p := &Prompt{Name: "p"}
	_, _ = p.AddVersion("a", "")
	_, _ = p.AddVersion("b", "")

	if err := p.Activate(1); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	got, _ := p.Active()
	if got.Version != 1 || got.Content != "a" {
		t.Fatalf("Active = %+v, want version 1", got)
	}
	if err := p.Activate(99); err != ErrVersionNotFound {
		t.Fatalf("want ErrVersionNotFound, got %v", err)
	}
}

func TestPromptPickVariantIsDeterministic(t *testing.T) {
	p := &Prompt{Name: "p"}
	_, _ = p.AddVersion("a", "")
	_, _ = p.AddVersion("b", "")
	_ = p.SetWeight(1, 50)
	_ = p.SetWeight(2, 50)

	seed := HashSeed("user-42")
	first, err := p.PickVariant(seed)
	if err != nil {
		t.Fatalf("PickVariant: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, _ := p.PickVariant(seed)
		if got.Version != first.Version {
			t.Fatalf("variant flapped: %d vs %d", got.Version, first.Version)
		}
	}
}

func TestPromptPickVariantDistributes(t *testing.T) {
	p := &Prompt{Name: "p"}
	_, _ = p.AddVersion("a", "")
	_, _ = p.AddVersion("b", "")
	_ = p.SetWeight(1, 50)
	_ = p.SetWeight(2, 50)

	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		v, _ := p.PickVariant(HashSeed(string(rune('a'+i%26)) + string(rune('0'+i/26))))
		seen[v.Version] = true
	}
	if len(seen) < 2 {
		t.Fatalf("all traffic landed on one variant: %v", seen)
	}
}

func TestPromptPickVariantFallsBackWhenNoWeights(t *testing.T) {
	p := &Prompt{Name: "p"}
	_, _ = p.AddVersion("a", "")
	_, _ = p.AddVersion("b", "")

	got, err := p.PickVariant(123)
	if err != nil {
		t.Fatalf("PickVariant: %v", err)
	}
	if got.Version != 2 {
		t.Fatalf("fallback version = %d, want 2", got.Version)
	}
}

func TestPromptPickVariantRespectsZeroWeight(t *testing.T) {
	p := &Prompt{Name: "p"}
	_, _ = p.AddVersion("a", "")
	_, _ = p.AddVersion("b", "")
	_ = p.SetWeight(1, 100)
	_ = p.SetWeight(2, 0)

	for i := 0; i < 50; i++ {
		v, _ := p.PickVariant(uint64(i))
		if v.Version != 1 {
			t.Fatalf("traffic leaked to zero-weight variant: %d", v.Version)
		}
	}
}

func TestPromptSetWeightErrors(t *testing.T) {
	p := &Prompt{Name: "p"}
	_, _ = p.AddVersion("a", "")
	if err := p.SetWeight(1, -1); err != ErrInvalidWeight {
		t.Fatalf("want ErrInvalidWeight, got %v", err)
	}
	if err := p.SetWeight(99, 10); err != ErrVersionNotFound {
		t.Fatalf("want ErrVersionNotFound, got %v", err)
	}
}

func TestHashSeedStable(t *testing.T) {
	if HashSeed("abc") != HashSeed("abc") {
		t.Fatal("HashSeed is not stable")
	}
	if HashSeed("abc") == HashSeed("abd") {
		t.Fatal("HashSeed collides on similar input")
	}
}