package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pdylanross/claude-workspace-manager/pkg/config"
)

// fixture stands in for a config document with more shape than Config has
// today: several leaf types, a nested group, and fields that are not part of
// the document at all.
type fixture struct {
	Name       string        `json:"name"`
	Count      int           `json:"count"`
	Limit      uint8         `json:"limit"`
	Ratio      float64       `json:"ratio"`
	Github     fixtureGithub `json:"github"`
	Omitted    string        `json:"-"`
	Legacy     string
	unexported string
}

type fixtureGithub struct {
	Enabled    bool   `json:"enabled"`
	RepoPrefix string `json:"repoPrefix"`
}

func newFixture() fixture {
	return fixture{
		Name:       "workspace",
		Count:      3,
		Limit:      7,
		Ratio:      1.5,
		Github:     fixtureGithub{Enabled: true, RepoPrefix: "cwm-"},
		Omitted:    "not in the document",
		Legacy:     "legacy",
		unexported: "also not in the document",
	}
}

func TestFieldPathsFlattensGroupsAndSkipsNonFields(t *testing.T) {
	t.Parallel()

	got := config.FieldPaths(reflect.TypeFor[fixture]())
	want := []string{"name", "count", "limit", "ratio", "github.enabled", "github.repoPrefix", "Legacy"}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("FieldPaths() = %v, want %v", got, want)
	}
}

func TestJSONName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		want  string
	}{
		{"a tagged field uses its tag", "Name", "name"},
		{"a dash means the field is not in the document", "Omitted", ""},
		{"an unexported field is not in the document", "unexported", ""},
		{"an untagged field uses its Go name", "Legacy", "Legacy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, ok := reflect.TypeFor[fixture]().FieldByName(tt.field)
			if !ok {
				t.Fatalf("fixture has no field %s", tt.field)
			}

			if got := config.JSONName(field); got != tt.want {
				t.Errorf("JSONName(%s) = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestCheckShapeAcceptsAnAddressableStruct(t *testing.T) {
	t.Parallel()

	if err := config.CheckShapeOf(reflect.TypeFor[fixture]()); err != nil {
		t.Errorf("CheckShapeOf(fixture) error = %v, want nil", err)
	}
}

func TestCheckShapeRejectsUnaddressableFields(t *testing.T) {
	t.Parallel()

	type withSlice struct {
		Things []string `json:"things"`
	}

	type withArray struct {
		Things [2]string `json:"things"`
	}

	type withMap struct {
		Things map[string]string `json:"things"`
	}

	type withPointer struct {
		Thing *string `json:"thing"`
	}

	type withAny struct {
		Thing any `json:"thing"`
	}

	type nestedSlice struct {
		Group struct {
			Things []int `json:"things"`
		} `json:"group"`
	}

	tests := []struct {
		name string
		typ  reflect.Type
		path string
	}{
		{"a slice", reflect.TypeFor[withSlice](), "things"},
		{"an array", reflect.TypeFor[withArray](), "things"},
		{"a map", reflect.TypeFor[withMap](), "things"},
		{"a pointer", reflect.TypeFor[withPointer](), "thing"},
		{"an interface", reflect.TypeFor[withAny](), "thing"},
		{"a slice inside a group", reflect.TypeFor[nestedSlice](), "group.things"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := config.CheckShapeOf(tt.typ)
			if err == nil {
				t.Fatal("CheckShapeOf() error = nil, want an error")
			}

			if !errors.Is(err, config.ErrUnsupportedKind) {
				t.Errorf("CheckShapeOf() error = %v, want it to wrap ErrUnsupportedKind", err)
			}

			if !strings.Contains(err.Error(), tt.path) {
				t.Errorf("CheckShapeOf() error = %q, want it to name %q", err, tt.path)
			}
		})
	}
}

func TestLookupField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want any
	}{
		{"a top-level setting", "name", "workspace"},
		{"a nested setting", "github.repoPrefix", "cwm-"},
		{"a nested boolean", "github.enabled", true},
		{"a group", "github", fixtureGithub{Enabled: true, RepoPrefix: "cwm-"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, err := config.LookupField(reflect.ValueOf(newFixture()), tt.path)
			if err != nil {
				t.Fatalf("LookupField(%q) error = %v", tt.path, err)
			}

			if got := field.Interface(); got != tt.want {
				t.Errorf("LookupField(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestLookupFieldRejectsBadPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantText string
	}{
		{"an empty path", "", ""},
		{"an unknown top-level setting", "nope", "known settings"},
		{"an unknown setting inside a group", "github.nope", "the settings under github"},
		{"a path through a single setting", "name.deeper", "has nothing beneath it"},
		{"a field that is not in the document", "Omitted", "known settings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.LookupField(reflect.ValueOf(newFixture()), tt.path)
			if err == nil {
				t.Fatalf("LookupField(%q) error = nil, want an error", tt.path)
			}

			if !errors.Is(err, config.ErrUnknownSetting) {
				t.Errorf("LookupField(%q) error = %v, want it to wrap ErrUnknownSetting", tt.path, err)
			}

			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("LookupField(%q) error = %q, want it to mention %q", tt.path, err, tt.wantText)
			}
		})
	}
}

func TestAssignField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		value string
		want  any
	}{
		{"a string is taken as written", "Name", "/srv/workspaces", "/srv/workspaces"},
		{"an empty string is allowed", "Name", "", ""},
		{"a signed number", "Count", "-12", -12},
		{"an unsigned number", "Limit", "200", uint8(200)},
		{"a float", "Ratio", "2.25", 2.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target := reflect.New(reflect.TypeFor[fixture]()).Elem()

			if err := config.AssignField(target.FieldByName(tt.field), tt.value); err != nil {
				t.Fatalf("AssignField() error = %v", err)
			}

			if got := target.FieldByName(tt.field).Interface(); got != tt.want {
				t.Errorf("AssignField(%q) left %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestAssignFieldParsesBooleans(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"true", "TRUE", "True", "1", "t"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			target := reflect.New(reflect.TypeFor[fixtureGithub]()).Elem()

			if err := config.AssignField(target.FieldByName("Enabled"), value); err != nil {
				t.Fatalf("AssignField(%q) error = %v", value, err)
			}

			if enabled, ok := reflect.TypeAssert[bool](target.FieldByName("Enabled")); !ok || !enabled {
				t.Errorf("AssignField(%q) left %v, want true", value, target.FieldByName("Enabled"))
			}
		})
	}
}

func TestAssignFieldRejectsBadValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		field    string
		value    string
		wantText string
	}{
		{"a number where a boolean belongs", "Github", "", ""},
		{"words where a number belongs", "Count", "many", "whole number"},
		{"a float where a whole number belongs", "Count", "1.5", "whole number"},
		{"a negative unsigned number", "Limit", "-1", "zero or more"},
		{"an unsigned number that overflows", "Limit", "256", "zero or more"},
		{"words where a float belongs", "Ratio", "lots", "not a number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target := reflect.New(reflect.TypeFor[fixture]()).Elem()

			err := config.AssignField(target.FieldByName(tt.field), tt.value)
			if err == nil {
				t.Fatalf("AssignField(%s=%q) error = nil, want an error", tt.field, tt.value)
			}

			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("AssignField(%s=%q) error = %q, want it to mention %q", tt.field, tt.value, err, tt.wantText)
			}
		})
	}
}

func TestAssignFieldRejectsABooleanValue(t *testing.T) {
	t.Parallel()

	target := reflect.New(reflect.TypeFor[fixtureGithub]()).Elem()

	err := config.AssignField(target.FieldByName("Enabled"), "maybe")
	if err == nil {
		t.Fatal("AssignField() error = nil, want an error")
	}

	if !strings.Contains(err.Error(), "true/false") {
		t.Errorf("AssignField() error = %q, want it to mention true/false", err)
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"a string is bare, so a shell can use it", "/srv/workspaces", "/srv/workspaces\n"},
		{"an empty string is an empty line", "", "\n"},
		{"a boolean", true, "true\n"},
		{"a number", 12, "12\n"},
		{
			name:  "a group is a JSON object",
			value: fixtureGithub{Enabled: true, RepoPrefix: "cwm-"},
			want:  "{\n  \"enabled\": true,\n  \"repoPrefix\": \"cwm-\"\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := config.Render(tt.value)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckShapeRejectsEmbeddedStructs(t *testing.T) {
	t.Parallel()

	type withEmbedded struct {
		fixtureGithub
	}

	// encoding/json would promote enabled and repoPrefix to the top level while
	// a path would address them under fixtureGithub, so the two would disagree.
	err := config.CheckShapeOf(reflect.TypeFor[withEmbedded]())
	if err == nil {
		t.Fatal("CheckShapeOf() error = nil, want an error")
	}

	if !errors.Is(err, config.ErrEmbeddedField) {
		t.Errorf("CheckShapeOf() error = %v, want it to wrap ErrEmbeddedField", err)
	}
}
