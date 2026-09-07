package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// pathSeparator joins the segments of a setting's path, as in
// "github.repoPrefix".
const pathSeparator = "."

// ErrUnknownSetting is returned when a path addresses nothing in the document.
var ErrUnknownSetting = errors.New("unknown setting")

// ErrNotASetting is returned when a path addresses a group of settings but a
// single value was required, as when setting "github" rather than a setting
// beneath it.
var ErrNotASetting = errors.New("not a single setting")

// ErrUnsupportedKind is returned when a field's type cannot be addressed: a
// slice, array, map, pointer, or anything else with no stable path to a single
// value. See [CheckShape].
var ErrUnsupportedKind = errors.New("unsupported field type")

// ErrEmbeddedField is returned when a config struct embeds another struct.
// [encoding/json] promotes an embedded struct's fields to the level above,
// while a path addresses them one level down, so the two would disagree about
// what a setting is called. Name the field instead of embedding it.
var ErrEmbeddedField = errors.New("embedded field")

// Settings returns the path of every setting in the document, in declaration
// order, with nested groups flattened ("github.enabled" rather than "github").
func Settings() []string {
	return fieldPaths(reflect.TypeFor[Config](), "")
}

// CheckShape reports whether the document can be addressed by path all the way
// down: nested structs to group settings, single values at the leaves.
//
// Slices, arrays and maps are rejected on purpose. A CLI that names settings by
// path has nothing to call the third element of a list, so a list in the
// document would be settable only by hand-editing the file — the thing this
// design exists to avoid. Model a list as named fields, or keep it out of the
// document.
func CheckShape() error {
	return checkShape(reflect.TypeFor[Config](), "")
}

// Render returns the printable form of a value from [Config.Get]: a group of
// settings as an indented JSON object, a single setting as its bare value with
// no quoting, so that shell callers can use it directly.
func Render(value any) ([]byte, error) {
	if reflect.ValueOf(value).Kind() == reflect.Struct {
		data, err := json.MarshalIndent(value, "", jsonIndent)
		if err != nil {
			return nil, fmt.Errorf("encode settings: %w", err)
		}

		return append(data, '\n'), nil
	}

	return fmt.Appendf(nil, "%v\n", value), nil
}

// Get returns the value at path: a single setting, or the struct holding a
// group of them.
func (c Config) Get(path string) (any, error) {
	field, err := lookupField(reflect.ValueOf(c), path)
	if err != nil {
		return nil, err
	}

	return field.Interface(), nil
}

// Set returns a copy of c with the setting at path parsed from value.
//
// The receiver is never modified, and the copy is a deep one because a config
// struct holds no reference types, so a caller applying several settings can
// discard the whole result if one of them fails.
func (c Config) Set(path, value string) (Config, error) {
	updated := c

	field, err := lookupField(reflect.ValueOf(&updated).Elem(), path)
	if err != nil {
		return Config{}, err
	}

	if field.Kind() == reflect.Struct {
		return Config{}, fmt.Errorf(
			"%w: %q is a group of settings; set one of: %s",
			ErrNotASetting, path, strings.Join(fieldPaths(field.Type(), path), ", "),
		)
	}

	if assignErr := assignField(field, value); assignErr != nil {
		return Config{}, fmt.Errorf("set %s: %w", path, assignErr)
	}

	return updated, nil
}

// fieldPaths returns the path of every setting under t, prefixed by prefix.
func fieldPaths(t reflect.Type, prefix string) []string {
	paths := make([]string, 0, t.NumField())

	for i := range t.NumField() {
		field := t.Field(i)

		name := jsonName(field)
		if name == "" {
			continue
		}

		path := joinPath(prefix, name)

		if field.Type.Kind() == reflect.Struct {
			paths = append(paths, fieldPaths(field.Type, path)...)

			continue
		}

		paths = append(paths, path)
	}

	return paths
}

// checkShape walks t and rejects any field that cannot be addressed by path.
func checkShape(t reflect.Type, prefix string) error {
	for i := range t.NumField() {
		field := t.Field(i)

		// Checked before anything else: an embedded field of an unexported type
		// still has its exported fields promoted into the document.
		if field.Anonymous {
			return fmt.Errorf(
				"%w: %s; give the field a name instead", ErrEmbeddedField, joinPath(prefix, field.Name),
			)
		}

		name := jsonName(field)
		if name == "" {
			continue
		}

		path := joinPath(prefix, name)

		if field.Type.Kind() == reflect.Struct {
			if err := checkShape(field.Type, path); err != nil {
				return err
			}

			continue
		}

		if !isSettable(field.Type) {
			return fmt.Errorf("%w: %s is a %s", ErrUnsupportedKind, path, field.Type.Kind())
		}
	}

	return nil
}

// lookupField walks path from v, one segment per separator.
func lookupField(v reflect.Value, path string) (reflect.Value, error) {
	if path == "" {
		return reflect.Value{}, fmt.Errorf("%w: %q", ErrUnknownSetting, path)
	}

	current := v
	walked := ""

	for segment := range strings.SplitSeq(path, pathSeparator) {
		if current.Kind() != reflect.Struct {
			return reflect.Value{}, fmt.Errorf(
				"%w: %q; %s is a single setting and has nothing beneath it",
				ErrUnknownSetting, path, walked,
			)
		}

		field, ok := structField(current, segment)
		if !ok {
			return reflect.Value{}, unknownSetting(path, walked, current.Type())
		}

		current = field
		walked = joinPath(walked, segment)
	}

	return current, nil
}

// structField returns the field of v whose on-disk name is name.
func structField(v reflect.Value, name string) (reflect.Value, bool) {
	t := v.Type()

	for i := range t.NumField() {
		if jsonName(t.Field(i)) == name {
			return v.Field(i), true
		}
	}

	return reflect.Value{}, false
}

// assignField parses value according to field's type and stores it.
func assignField(field reflect.Value, value string) error {
	switch {
	case field.Kind() == reflect.String:
		field.SetString(value)
	case field.Kind() == reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%q is not a true/false value: %w", value, err)
		}

		field.SetBool(parsed)
	case field.CanInt():
		parsed, err := strconv.ParseInt(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a whole number: %w", value, err)
		}

		field.SetInt(parsed)
	case field.CanUint():
		parsed, err := strconv.ParseUint(value, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a whole number that is zero or more: %w", value, err)
		}

		field.SetUint(parsed)
	case field.CanFloat():
		parsed, err := strconv.ParseFloat(value, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not a number: %w", value, err)
		}

		field.SetFloat(parsed)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedKind, field.Kind())
	}

	return nil
}

// isSettable reports whether a field of type t holds one value that
// [assignField] can parse from a string.
func isSettable(t reflect.Type) bool {
	zero := reflect.Zero(t)

	return zero.Kind() == reflect.String || zero.Kind() == reflect.Bool ||
		zero.CanInt() || zero.CanUint() || zero.CanFloat()
}

// jsonName returns the on-disk name of field, or "" when the field is not part
// of the document.
func jsonName(field reflect.StructField) string {
	if !field.IsExported() {
		return ""
	}

	tag, tagged := field.Tag.Lookup("json")
	if !tagged {
		return field.Name
	}

	name, _, _ := strings.Cut(tag, ",")

	switch name {
	case "-":
		return ""
	case "":
		return field.Name
	default:
		return name
	}
}

// joinPath appends segment to prefix, which may be empty at the top level.
func joinPath(prefix, segment string) string {
	if prefix == "" {
		return segment
	}

	return prefix + pathSeparator + segment
}

// unknownSetting reports a path that addresses nothing, naming what the group
// it failed in does hold so the user can correct the path.
func unknownSetting(path, walked string, group reflect.Type) error {
	known := strings.Join(fieldPaths(group, walked), ", ")

	if walked == "" {
		return fmt.Errorf("%w: %q; known settings: %s", ErrUnknownSetting, path, known)
	}

	return fmt.Errorf("%w: %q; the settings under %s: %s", ErrUnknownSetting, path, walked, known)
}
