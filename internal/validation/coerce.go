package validation

import (
	"fmt"
	"strconv"
)

func Coerce(expected string, val any) (any, error) {
	switch expected {
	case "int":
		switch v := val.(type) {
		case float64:
			return int(v), nil
		case string:
			return strconv.Atoi(v)
		}
	case "string":
		return fmt.Sprintf("%v", val), nil
	case "bool":
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			return strconv.ParseBool(v)
		}
	}
	return val, nil
}
