package utils

import "testing"

func TestEnvBoolRecognizesTruthyValues(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "YES", "on", "On"} {
		t.Setenv("MC_AGENT_TEST_ENVBOOL", v)
		if !envBool("MC_AGENT_TEST_ENVBOOL") {
			t.Fatalf("envBool(%q) = false, want true", v)
		}
	}
}

func TestEnvBoolRejectsUnsetOrOtherValues(t *testing.T) {
	t.Setenv("MC_AGENT_TEST_ENVBOOL", "")
	if envBool("MC_AGENT_TEST_ENVBOOL") {
		t.Fatalf("envBool(unset) = true, want false")
	}
	for _, v := range []string{"0", "false", "no", "off", "garbage"} {
		t.Setenv("MC_AGENT_TEST_ENVBOOL", v)
		if envBool("MC_AGENT_TEST_ENVBOOL") {
			t.Fatalf("envBool(%q) = true, want false", v)
		}
	}
}
