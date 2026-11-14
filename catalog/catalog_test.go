package catalog

import "testing"

func TestLoad(t *testing.T) {
	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("empty catalog")
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].IntroducedIn > all[i].IntroducedIn {
			t.Fatal("catalog not sorted by version")
		}
	}
}

func TestInRange(t *testing.T) {
	all := []Entry{{ID: "PG13-a", IntroducedIn: 13}, {ID: "PG15-b", IntroducedIn: 15}, {ID: "PG18-c", IntroducedIn: 18}}
	got := InRange(all, 13, 15)
	if len(got) != 1 || got[0].ID != "PG15-b" {
		t.Errorf("InRange(13,15) = %+v", got)
	}
	if got := InRange(all, 12, 18); len(got) != 3 {
		t.Errorf("InRange(12,18) returned %d entries", len(got))
	}
}

func TestValidateRejectsBadEntries(t *testing.T) {
	good := Entry{
		ID: "PG15-x", IntroducedIn: 15, Severity: Error, Title: "t", Remediation: "r",
		URL: "https://www.postgresql.org/docs/release/15.0/", Detect: Detect{Always: true},
	}
	if err := validate(good); err != nil {
		t.Fatalf("good entry rejected: %v", err)
	}
	bad := []func(e *Entry){
		func(e *Entry) { e.ID = "PG14-x" },
		func(e *Entry) { e.Severity = "fatal" },
		func(e *Entry) { e.URL = "https://example.com/" },
		func(e *Entry) { e.Detect = Detect{} },
		func(e *Entry) { e.Detect = Detect{SQLRegex: []string{"("}} },
	}
	for i, mutate := range bad {
		e := good
		mutate(&e)
		if validate(e) == nil {
			t.Errorf("bad entry %d accepted", i)
		}
	}
}
