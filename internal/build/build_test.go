package build

import (
	"encoding/json"
	"strings"
	"testing"

	"node-box/internal/model"
	"node-box/internal/output"
)

func mods(m map[string]string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = json.RawMessage(v)
	}
	return out
}

func outputs(cfs ...model.ConfigFile) []output.Target {
	var res []output.Target
	for _, cf := range cfs {
		res = append(res, output.Target{Config: cf, Path: "/out/" + cf.Name + ".json"})
	}
	return res
}

func TestBuild_MergesModulesInOrder(t *testing.T) {
	cf := model.ConfigFile{Name: "main", Path: "main.json", Modules: []string{"log", "dns"}}

	files, err := Build(Input{
		Config: &model.Config{},
		Modules: mods(map[string]string{
			"log": `{"log":{"level":"info"}}`,
			"dns": `{"dns":{"servers":[{"tag":"local"}]}}`,
		}),
		Outputs: outputs(cf),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	var doc map[string]any
	if err := json.Unmarshal(files[0].Content, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := doc["log"]; !ok {
		t.Error("log section missing")
	}
	if _, ok := doc["dns"]; !ok {
		t.Error("dns section missing")
	}
	if files[0].Path != "/out/main.json" {
		t.Errorf("path = %q, want /out/main.json", files[0].Path)
	}
	if !strings.HasSuffix(string(files[0].Content), "}\n") {
		t.Error("output should end with a newline")
	}
}

func TestBuild_ConflictingKeysAreRejected(t *testing.T) {
	cf := model.ConfigFile{Name: "main", Modules: []string{"a", "b"}}

	_, err := Build(Input{
		Config: &model.Config{},
		Modules: mods(map[string]string{
			"a": `{"route":{"rules":[]},"log":{"level":"info"}}`,
			"b": `{"route":{"final":"direct"}}`,
		}),
		Outputs: outputs(cf),
	})
	if err == nil {
		t.Fatal("want an error when two modules define the same key")
	}
	// The message must name both modules and the key, otherwise it is not
	// actionable.
	for _, want := range []string{`"a"`, `"b"`, `"route"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %s", err, want)
		}
	}
}

func TestBuild_RejectsNonObjectModule(t *testing.T) {
	for name, content := range map[string]string{
		"array":  `[{"tag":"a"}]`,
		"string": `"hello"`,
		"empty":  ``,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Build(Input{
				Config:  &model.Config{},
				Modules: mods(map[string]string{"m": content}),
				Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"m"}}),
			})
			if err == nil {
				t.Fatalf("want an error for a %s module", name)
			}
		})
	}
}

func TestBuild_MissingModule(t *testing.T) {
	_, err := Build(Input{
		Config:  &model.Config{},
		Modules: mods(map[string]string{}),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"nope"}}),
	})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("want an error naming the missing module, got %v", err)
	}
}

func TestBuild_DropsEmptySections(t *testing.T) {
	files, err := Build(Input{
		Config: &model.Config{},
		Modules: mods(map[string]string{
			"m": `{"log":{"level":"info"},"route":{},"endpoints":[],"ntp":null}`,
		}),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"m"}}),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(files[0].Content, &doc)

	if _, ok := doc["log"]; !ok {
		t.Error("log should be kept")
	}
	for _, key := range []string{"route", "endpoints", "ntp"} {
		if _, ok := doc[key]; ok {
			t.Errorf("empty section %q should have been dropped", key)
		}
	}
}

func TestBuild_AllSectionsEmptyIsAnError(t *testing.T) {
	_, err := Build(Input{
		Config:  &model.Config{},
		Modules: mods(map[string]string{"m": `{"route":{},"outbounds":[]}`}),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"m"}}),
	})
	if err == nil {
		t.Fatal("want an error when the generated configuration is empty")
	}
}

func TestBuild_IsDeterministic(t *testing.T) {
	in := Input{
		Config: &model.Config{},
		Modules: mods(map[string]string{
			"m": `{"zulu":{"a":1},"alpha":{"b":2},"mike":{"c":3}}`,
		}),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"m"}}),
	}

	first, err := Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Repeat enough times that a map-iteration dependency would show up.
	for i := range 20 {
		again, err := Build(in)
		if err != nil {
			t.Fatalf("Build %d: %v", i, err)
		}
		if string(again[0].Content) != string(first[0].Content) {
			t.Fatalf("run %d produced different bytes:\n%s\n---\n%s", i, first[0].Content, again[0].Content)
		}
		if again[0].Hash() != first[0].Hash() {
			t.Fatalf("run %d produced a different hash", i)
		}
	}
}

func TestBuild_PassthroughLeavesOutboundsAlone(t *testing.T) {
	files, err := Build(Input{
		Config: &model.Config{},
		Modules: mods(map[string]string{
			"m": `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"sel","outbounds":["direct"]}]}`,
		}),
		Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"m"}}),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(files[0].Content, &doc)
	obs, ok := doc["outbounds"].([]any)
	if !ok || len(obs) != 2 {
		t.Fatalf("outbounds = %v, want the 2 the module declared", doc["outbounds"])
	}
}

func TestBuild_RejectsMalformedSections(t *testing.T) {
	tests := map[string]string{
		"outbounds not an array":   `{"outbounds":{"tag":"a"}}`,
		"outbounds holds a scalar": `{"outbounds":["direct"]}`,
		"inbounds not an array":    `{"inbounds":"mixed"}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Build(Input{
				Config:  &model.Config{},
				Modules: mods(map[string]string{"m": content}),
				Outputs: outputs(model.ConfigFile{Name: "main", Modules: []string{"m"}}),
			})
			if err == nil {
				t.Fatal("want a validation error")
			}
		})
	}
}

func TestBuild_MultipleOutputs(t *testing.T) {
	files, err := Build(Input{
		Config: &model.Config{},
		Modules: mods(map[string]string{
			"log":   `{"log":{"level":"info"}}`,
			"dns":   `{"dns":{"servers":[]}}`,
			"route": `{"route":{"final":"direct"}}`,
		}),
		Outputs: outputs(
			model.ConfigFile{Name: "main", Modules: []string{"log", "route"}},
			model.ConfigFile{Name: "alt", Modules: []string{"log"}},
		),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].Name != "main" || files[1].Name != "alt" {
		t.Errorf("outputs should keep config order, got %q then %q", files[0].Name, files[1].Name)
	}
	// Each output only contains the modules it asked for.
	var alt map[string]any
	json.Unmarshal(files[1].Content, &alt)
	if _, ok := alt["route"]; ok {
		t.Error("alt should not contain route")
	}
}
