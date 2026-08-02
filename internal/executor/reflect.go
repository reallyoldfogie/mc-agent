package executor

import (
	"context"
	"fmt"
	"reflect"

	"github.com/reallyoldfogie/mc-agent/internal/validation"
)

type ReflectExecutor struct {
	Target any
	Spec   map[string]map[string]string // method -> param types
}

func (r *ReflectExecutor) Execute(ctx context.Context, name string, args map[string]any) (any, error) {
	v := reflect.ValueOf(r.Target)
	method := v.MethodByName(name)

	if !method.IsValid() {
		return nil, fmt.Errorf("method %s not found", name)
	}

	expected := r.Spec[name]

	in := []reflect.Value{}
	for param, typ := range expected {
		val, ok := args[param]
		if !ok {
			return nil, fmt.Errorf("missing param %s", param)
		}

		coerced, err := validation.Coerce(typ, val)
		if err != nil {
			return nil, err
		}

		in = append(in, reflect.ValueOf(coerced))
	}

	out := method.Call(in)

	if len(out) == 0 {
		return nil, nil
	}

	return out[0].Interface(), nil
}
