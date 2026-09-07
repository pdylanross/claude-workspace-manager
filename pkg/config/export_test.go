package config

import "reflect"

// FieldPaths exposes the path walker to the config_test package, so that the
// nesting rules can be tested against fixture types rather than only against
// whatever Config happens to hold today.
func FieldPaths(t reflect.Type) []string {
	return fieldPaths(t, "")
}

// CheckShapeOf exposes the shape check for an arbitrary type.
func CheckShapeOf(t reflect.Type) error {
	return checkShape(t, "")
}

// LookupField exposes the path walker.
func LookupField(v reflect.Value, path string) (reflect.Value, error) {
	return lookupField(v, path)
}

// AssignField exposes the string-to-field parser.
func AssignField(field reflect.Value, value string) error {
	return assignField(field, value)
}

// JSONName exposes the on-disk field naming.
func JSONName(field reflect.StructField) string {
	return jsonName(field)
}
