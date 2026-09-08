package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// ApplyEnv overlays environment variables onto settings — the middle layer
// of JSON -> ENV -> CLI precedence. This is the one piece neither
// predecessor (the old auth-only config.AppConfig, rlconfig.Settings) had:
// a single generic implementation here, not a per-field switch statement
// hand-written again for every section, is what actually stops this
// package from fragmenting the next time a section grows — see
// docs/plans/UNIFIED_CONFIG_PLAN.md.
//
// For each leaf field reachable through nested structs, the corresponding
// env var name is "<prefix>_" followed by every enclosing struct field's
// json tag name and the leaf field's own json tag name, each uppercased
// and joined with "_" — e.g. Settings.Connection.Address's json path
// ("connection", "address") becomes "<prefix>_CONNECTION_ADDRESS".
//
// Array/slice fields are skipped (only EnvSettings.TargetOffset today) —
// not worth generic CSV-parsing support for one field; override those via
// JSON or CLI instead. A field with no env var set (os.LookupEnv's second
// return is false) is left untouched, matching JSON's own "only overwrite
// fields present in the file" contract and CLI's own "only override
// explicitly-passed flags" contract — an unset layer never clobbers a set
// one beneath it.
func ApplyEnv(settings *Settings, prefix string) error {
	return applyEnvStruct(reflect.ValueOf(settings).Elem(), prefix)
}

func applyEnvStruct(v reflect.Value, prefix string) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			name = field.Name
		}
		envName := prefix + "_" + strings.ToUpper(name)
		fv := v.Field(i)

		switch fv.Kind() {
		case reflect.Struct:
			if err := applyEnvStruct(fv, envName); err != nil {
				return err
			}
			continue
		case reflect.Array, reflect.Slice:
			continue // not supported generically — see doc comment
		}

		raw, ok := os.LookupEnv(envName)
		if !ok {
			continue
		}
		if err := setFieldFromString(fv, raw); err != nil {
			return fmt.Errorf("config: env %s: %w", envName, err)
		}
	}
	return nil
}

func setFieldFromString(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return err
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return err
		}
		fv.SetFloat(f)
	default:
		return fmt.Errorf("unsupported field kind %s", fv.Kind())
	}
	return nil
}
