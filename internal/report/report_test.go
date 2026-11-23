package report

import "testing"

func TestNormalizeDropsVolatileFields(t *testing.T) {
	got, err := Normalize([]byte(`{"verdict":"PASSED","duration_ms":8,"timestamp":"2026-11-23T00:00:00Z","exit_code":0,"findings":[{"rule_id":"helm.values.new_required_unset","severity":"blocker","ignored":false},{"rule_id":"ignored.rule","severity":"blocker","ignored":true},{"rule_id":"note","severity":"info"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(map[string]any)
	if _, ok := obj["duration_ms"]; ok {
		t.Fatal("duration_ms remains")
	}
	if _, ok := obj["timestamp"]; ok {
		t.Fatal("timestamp remains")
	}
	if Verdict(got) != "PASSED" {
		t.Fatal(Verdict(got))
	}
	rules := BlockingRules(got)
	if len(rules) != 1 || rules[0] != "helm.values.new_required_unset" {
		t.Fatalf("%#v", rules)
	}
	buf, err := Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf) != string(again) {
		t.Fatalf("marshal drifted\n%s\n%s", buf, again)
	}
}
