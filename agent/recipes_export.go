package agent

import (
	"encoding/json"
)

// ExportLastUpdateRecipesAsJSON returns the last parsed Update Recipes payload as JSON.
// If indent is true, the JSON will be pretty-printed.
func (a *agent) ExportLastUpdateRecipesAsJSON(indent bool) (string, bool, error) {
	a.recipesMu.RLock()
	payload := a.lastUpdateRecipes
	a.recipesMu.RUnlock()
	if payload == nil {
		return "", false, nil
	}
	var (
		b   []byte
		err error
	)
	if indent {
		b, err = json.MarshalIndent(payload, "", "  ")
	} else {
		b, err = json.Marshal(payload)
	}
	if err != nil {
		return "", true, err
	}
	return string(b), true, nil
}
